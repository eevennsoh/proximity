package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strings"
	"text/template"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

type monifyRequestFn func(*http.Request)
type modifyResponseFn func(*http.Response) error

type endpointProxyConfig struct {
	baseEndpoint *url.URL

	config.UriMap
	config.RequestResponse
}

func (s *server) logRequest(req *http.Request) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Fatal(err)
	}

	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	reqCopy, err := copyRequest(req)
	if err != nil {
		log.Fatal(err)
	}

	reqBytes, err := json.Marshal(reqCopy)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("REQUEST:", string(reqBytes))
}

func (s *server) logResponse(res *http.Response) {
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	resCopy, err := copyResponse(res)
	if err != nil {
		log.Fatal(err)
	}

	reqBytes, err := json.Marshal(resCopy)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("RESPONSE:", string(reqBytes))
}

func (s *server) modifyResponse(cfg *endpointProxyConfig) modifyResponseFn {
	return func(res *http.Response) error {
		contentType := res.Header.Get("Content-Type")

		// If we're not getting a stream back then just log out the response
		// and stop there.
		if !strings.HasPrefix(contentType, "text/event-stream") {
			s.logResponse(res)
			return nil
		}

		pr, pw := io.Pipe()
		orig := res.Body

		go func() {
			defer orig.Close()
			defer pw.Close()

			reader := bufio.NewReader(orig)

			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					log.Println(err)
					break
				}

				if len(line) == 0 {
					continue
				}

				// Modify the line as needed here
				modifiedLine, err := processSseLine(line)
				if err != nil {
					log.Println(err)
					break
				}

				if _, err := pw.Write([]byte(modifiedLine)); err != nil {
					log.Println(err)
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
func processSseLine(line string) (string, error) {
	newLine := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

	if len(newLine) == 0 {
		return line, nil
	}

	var event map[string]any

	if err := json.Unmarshal([]byte(newLine), &event); err != nil {
		log.Println("warning: could not unmarshal SSE event:", err)
		return line, nil
	}

	_, ok := event["amazon-bedrock-invocationMetrics"]
	if ok {
		delete(event, "amazon-bedrock-invocationMetrics")
	}

	message, ok := event["message"]
	if ok {
		messageMap := message.(map[string]any)

		usage, ok := messageMap["usage"]
		if ok {
			usageMap := usage.(map[string]any)

			messageMap["usage"] = map[string]any{
				"input_tokens":  usageMap["input_tokens"],
				"output_tokens": usageMap["output_tokens"],
			}
		}
	}

	// if event["type"].(string) == "content_block_start" {
	// 	contentBlockMap := event["content_block"].(map[string]any)
	// 	contentBlockMap["text"] = "hello world"
	// }

	// val, ok := event["usage"]
	// if ok {
	// 	fmt.Println("GOING TO CHANGE LINE:", newLine)

	// 	event["usage"] = map[string]string{
	// 		"input_tokens":  val.(map[string]string)["input_tokens"],
	// 		"output_tokens": val.(map[string]string)["output_tokens"],
	// 	}
	// }

	bytes, err := json.Marshal(event)
	if err != nil {
		return "", err
	}

	log.Println(string(bytes))

	// Rebuild the line and make sure to keep the formatting the same
	return fmt.Sprintf("data: %s\n", bytes), nil
}

func (s *server) modifyRequest(cfg *endpointProxyConfig, originalDirector func(*http.Request)) monifyRequestFn {
	return func(req *http.Request) {
		originalDirector(req)
		s.logRequest(req)

		// // Make a deep copy of the request for safe rendering and extraction
		// originalRequest := copyRequest(req)

		// if cfg.Out != "" {
		// 	renderedPath := renderOutURI(cfg.Out, originalRequest)
		// 	req.URL.Path = renderedPath
		// 	req.RequestURI = renderedPath
		// }

		// // Render headers as they would be after patching
		// renderedHeaders := http.Header{}
		// for k, v := range req.Header {
		// 	copied := make([]string, len(v))
		// 	copy(copied, v)
		// 	renderedHeaders[k] = copied
		// }
		// for _, header := range cfg.Headers {
		// 	_ = s.overrideHeader(req, header, originalRequest)
		// }

		// // Render body as it would be after patching
		// var renderedBody any
		// if req.Body != nil && req.Header.Get("Content-Type") == "application/json" {
		// 	bodyBytes, _ := io.ReadAll(req.Body)
		// 	_ = json.Unmarshal(bodyBytes, &renderedBody)
		// 	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		// }

		// log.Println(req.Method, req.RequestURI, req.Header.Get("User-Agent"))

		// if err := s.overrideBody(req, cfg.Body); err != nil {
		// 	log.Fatal(err)
		// }
	}
}

// An endpoint proxy handles proxying a single URI
// The config will have been generated for it be combining the global and uri override configs together
// There is a generic function for an endpoint proxy which takes in config to work
// The config determines how the endpoint proxy operates

func (s *server) handleEndpoint(cfg *endpointProxyConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.RequestURI, r.Header.Get("User-Agent"))

		proxyHandler := s.endpointProxy(cfg)

		if err := s.renderRequest(r, cfg); err != nil {
			log.Fatal(err)
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

	proxy.Director = s.modifyRequest(cfg, proxy.Director)
	proxy.ModifyResponse = s.modifyResponse(cfg)

	return proxy
}

func (s *server) serveRenderedRequest(w http.ResponseWriter, r *http.Request) {
	reqCopy, err := copyRequest(r)
	if err != nil {
		log.Fatal(err)
	}

	bodyMap, err := extractJsonBody(reqCopy)
	if err != nil {
		log.Fatal(err)
	}

	reqCopy.Body = bodyMap

	w.Header().Set("Content-Type", "application/json")

	pretty, err := json.MarshalIndent(reqCopy, "", "  ")
	if err != nil {
		log.Fatal(err)
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

func (s *server) overrideBody(originalReq *http.Request, copiedReq *HttpRequest, body config.Body) error {
	if body.Template != "" {
		renderedBodyBytes, err := s.renderTemplateString(strings.TrimSpace(body.Template), copiedReq)
		if err != nil {
			return err
		}

		s.applyNewBodyToRequest(originalReq, renderedBodyBytes)
		return nil
	}

	if len(body.Patches) == 0 {
		return nil
	}

	bodyBytes, err := io.ReadAll(originalReq.Body)
	if err != nil {
		return err
	}

	newBody, err := s.applyPatchToJson(body.Patches, bodyBytes)
	if err != nil {
		return err
	}

	s.applyNewBodyToRequest(originalReq, newBody)
	return nil
}

func (s *server) renderTemplateString(templateStr string, req *HttpRequest) ([]byte, error) {
	jsonBody, err := extractJsonBody(req)
	if err != nil {
		return nil, err
	}

	templateInput := map[string]any{
		"body":    jsonBody,
		"headers": req.Headers,
	}

	funcMap := template.FuncMap{
		"toJson": func(v any) string {
			b, err := json.Marshal(v)
			if err != nil {
				log.Fatal(err)
			}

			return string(b)
		},
		"getType": func(v any) string {
			return reflect.TypeOf(v).Kind().String()
		},
		"safeEncode": func(v any) string {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				log.Fatal(err)
			}

			safeString := string(jsonBytes)
			safeString, _ = strings.CutPrefix(safeString, "\"")
			safeString, _ = strings.CutSuffix(safeString, "\"")
			return safeString
		},
		"normalizeModel": func(model, prefix, suffix string) string {
			model = strings.TrimPrefix(model, prefix)
			model = strings.TrimSuffix(model, suffix)
			return prefix + model + suffix
		},
	}

	tmpl, err := template.New("body").Funcs(funcMap).Parse(templateStr)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, templateInput); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// extractJsonBody extracts the JSON body from the request as a map[string]any.
func extractJsonBody(req *HttpRequest) (map[string]any, error) {
	if req.Headers.Get("Content-Type") == "" || req.Headers.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("content type is %s, expected application/json", req.Headers.Get("Content-Type"))
	}

	if req.Body == nil {
		return map[string]any{}, nil
	}

	var bodyMap map[string]any

	if err := json.Unmarshal(req.Body.([]byte), &bodyMap); err != nil {
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
