package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"text/template"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

type modifyResponseFn func(*http.Response) error

type endpointProxyConfig struct {
	baseEndpoint *url.URL

	config.UriMap
	config.RequestResponse
}

func (s *server) modifyResponse(cfg *endpointProxyConfig) modifyResponseFn {
	return func(res *http.Response) error {
		contentType := res.Header.Get("Content-Type")

		// If we're not getting a stream back then just log out the response
		// and stop there.
		if !strings.HasPrefix(contentType, "text/event-stream") {
			return nil
		}

		pr, pw := io.Pipe()
		orig := res.Body

		go func() {
			defer orig.Close()
			defer pw.Close()

			reader := bufio.NewReader(orig)

			renderStorage := make(map[string]string)

			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					s.Logger.Println(err)
					break
				}

				if len(line) == 0 {
					continue
				}

				// Modify the line as needed here
				modifiedLine, err := s.processSseLine(line, cfg.Response.Body, renderStorage)
				if err != nil {
					s.Logger.Println(err)
					break
				}

				if _, err := pw.Write([]byte(modifiedLine)); err != nil {
					s.Logger.Println(err)
					break
				}
			}
		}()

		res.Body = pr
		return nil
	}
}

// processSseLine allows you to modify each SSE line as it comes through.
// For now, it just logs and returns the line unmodified.
func (s *server) processSseLine(line string, bodyOverride config.Body, renderStorage map[string]string) (string, error) {
	newLine := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

	if newLine == "" {
		return line, nil
	}

	// No overrides defined, return as is
	if bodyOverride.Template == "" {
		return line, nil
	}

	var event map[string]any

	if err := json.Unmarshal([]byte(newLine), &event); err != nil {
		s.Logger.Println("warning: could not unmarshal SSE event:", err)
		return line, nil
	}

	templateInput := map[string]any{
		"event": event,
	}

	renderedEventBytes, err := s.renderTemplateString(strings.TrimSpace(bodyOverride.Template), templateInput, renderStorage)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	if err := json.Compact(&buf, renderedEventBytes); err != nil {
		return "", err
	}

	// Rebuild the line and make sure to keep the formatting the same
	return fmt.Sprintf("data: %s\n", buf.String()), nil
}

// An endpoint proxy handles proxying a single URI
// The config will have been generated for it be combining the global and uri override configs together
// There is a generic function for an endpoint proxy which takes in config to work
// The config determines how the endpoint proxy operates

func (s *server) handleEndpoint(cfg *endpointProxyConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.Logger.Println(r.Method, r.RequestURI, r.Header.Get("User-Agent"))

		proxyHandler := s.endpointProxy(cfg)

		if err := s.renderRequest(r, cfg); err != nil {
			s.Logger.Fatal(err)
		}

		if s.TestMode {
			s.serveRenderedRequest(w, r)
			return
		}

		proxyHandler.ServeHTTP(w, r)
	}
}

func (s *server) endpointProxy(cfg *endpointProxyConfig) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(cfg.baseEndpoint)

	// proxy.Director = s.modifyRequest(cfg, proxy.Director)
	proxy.ModifyResponse = s.modifyResponse(cfg)

	return proxy
}

func (s *server) serveRenderedRequest(w http.ResponseWriter, r *http.Request) {
	reqCopy, err := copyRequest(r)
	if err != nil {
		s.Logger.Fatal(err)
	}

	bodyMap, err := extractJsonBody(reqCopy.Headers, reqCopy.Body)
	if err != nil {
		s.Logger.Fatal(err)
	}

	reqCopy.Body = bodyMap

	w.Header().Set("Content-Type", "application/json")

	pretty, err := json.MarshalIndent(reqCopy, "", "  ")
	if err != nil {
		s.Logger.Fatal(err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, string(pretty))
}

func (s *server) makeRequest(request config.Request) ([]byte, error) {
	req, err := http.NewRequest(request.Method, request.Url, bytes.NewBuffer([]byte(request.JsonBody)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (s *server) getValueAtPath(data any, path string) (string, error) {
	pathElements := strings.Split(strings.Trim(path, "/"), "/")

	for _, key := range pathElements {
		switch curr := data.(type) {
		case map[string]any:
			data = curr[key]
		default:
			return "", fmt.Errorf("error parsing response")
		}
	}

	strValue, ok := data.(string)
	if !ok {
		return "", fmt.Errorf("error parsing response, returned type is not a string: %v", strValue)
	}

	return strValue, nil
}

func (s *server) overrideRequestBody(originalReq *http.Request, copiedReq *HttpRequest, bodyOverride config.Body) error {
	if bodyOverride.Template != "" {
		templateInput, err := s.buildTemplateInput(copiedReq.Headers, copiedReq.Body)
		if err != nil {
			return err
		}

		renderedBodyBytes, err := s.renderTemplateString(strings.TrimSpace(bodyOverride.Template), templateInput, nil)
		if err != nil {
			return err
		}

		var buf bytes.Buffer

		if err := json.Compact(&buf, renderedBodyBytes); err != nil {
			return err
		}

		s.applyNewBodyToRequest(originalReq, buf.Bytes())
		return nil
	}

	if len(bodyOverride.Patches) == 0 {
		return nil
	}

	bodyBytes, err := io.ReadAll(originalReq.Body)
	if err != nil {
		return err
	}

	newBody, err := s.applyPatchToJson(bodyOverride.Patches, bodyBytes)
	if err != nil {
		return err
	}

	s.applyNewBodyToRequest(originalReq, newBody)
	return nil
}

// func (s *server) overrideResponseBody(originalRes *http.Response, copiedRes *HttpResponse, bodyOverride config.Body) error {
// 	if bodyOverride.Template != "" {
// 		templateInput, err := s.buildTemplateInput(copiedRes.Headers, copiedRes.Body)
// 		if err != nil {
// 			return err
// 		}

// 		renderedBodyBytes, err := s.renderTemplateString(strings.TrimSpace(bodyOverride.Template), templateInput)
// 		if err != nil {
// 			return err
// 		}

// 		s.applyNewBodyToResponse(originalRes, renderedBodyBytes)
// 		return nil
// 	}

// 	if len(bodyOverride.Patches) == 0 {
// 		return nil
// 	}

// 	bodyBytes, err := io.ReadAll(originalRes.Body)
// 	if err != nil {
// 		return err
// 	}

// 	newBody, err := s.applyPatchToJson(bodyOverride.Patches, bodyBytes)
// 	if err != nil {
// 		return err
// 	}

// 	s.applyNewBodyToResponse(originalRes, newBody)
// 	return nil
// }

func (s *server) renderTemplateString(templateStr string, templateInput map[string]any, storage map[string]string) ([]byte, error) {
	tmpl, err := template.New("body").Funcs(s.template.functionsWithStorage(storage)).Parse(templateStr)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, templateInput); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *server) buildTemplateInput(headers http.Header, body any) (map[string]any, error) {
	jsonBody, err := extractJsonBody(headers, body)
	if err != nil {
		return nil, err
	}

	templateInput := map[string]any{
		"body":    jsonBody,
		"headers": headers,
	}

	return templateInput, nil
}

// extractJsonBody extracts the JSON body from the request/response as a map[string]any.
func extractJsonBody(headers http.Header, body any) (map[string]any, error) {
	if headers.Get("Content-Type") == "" || headers.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("content type is %s, expected application/json", headers.Get("Content-Type"))
	}

	if body == nil {
		return map[string]any{}, nil
	}

	var bodyMap map[string]any

	if err := json.Unmarshal(body.([]byte), &bodyMap); err != nil {
		return nil, err
	}

	return bodyMap, nil
}

func (s *server) applyPatchToJson(patchData []config.Patch, bodyBytes []byte) ([]byte, error) {
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		return nil, err
	}

	newBody, err := patch.Apply(bodyBytes)
	if err != nil {
		return nil, err
	}

	return newBody, nil
}

func (s *server) applyNewBodyToRequest(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

// func (s *server) applyNewBodyToResponse(res *http.Response, body []byte) {
// 	res.Body = io.NopCloser(bytes.NewReader(body))
// 	res.ContentLength = int64(len(body))

// 	if res.Header != nil {
// 		res.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
// 		res.Header.Del("Transfer-Encoding")
// 	}
// }
