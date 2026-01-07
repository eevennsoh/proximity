package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
)

type HttpRequest struct {
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Headers http.Header `json:"headers"`
	Body    any         `json:"body,omitempty"`
}

type HttpResponse struct {
	Headers http.Header `json:"headers"`
	Body    any         `json:"body,omitempty"`
}

// renderRequest applies all config-driven transformations to the request and returns a new http.Request.
func (s *server) renderRequest(req *http.Request, cfg *endpointProxyConfig, templateInput map[string]any) error {
	if !cfg.Out.IsEmpty() {
		renderedPath := cfg.Out.Text

		if cfg.Out.Text == "" {
			// Use unified render to support both Template and Expr
			renderedPathBytes, err := s.renderer.Render(cfg.Out.Template, cfg.Out.Expr, templateInput, nil)
			if err != nil {
				return err
			}

			renderedPath = strings.TrimSpace(string(renderedPathBytes))
		}

		req.URL.Path = renderedPath
		req.RequestURI = renderedPath
	}

	if err := s.overrideHeaders(cfg.Request.Headers, &req.Header, templateInput); err != nil {
		return err
	}

	// Apply body patches/overrides as per config
	if err := s.overrideRequestBody(req, templateInput, cfg.Request.Body); err != nil {
		return fmt.Errorf("error applying body override: %v", err)
	}

	return nil
}

// renderResponse applies all config-driven transformations to the response and returns a new http.Reponse.
func (s *server) renderResponse(res *http.Response, cfg *endpointProxyConfig, templateInput map[string]any) error {
	if err := s.overrideHeaders(cfg.Response.Headers, &res.Header, templateInput); err != nil {
		return err
	}

	// Apply body patches/overrides as per config
	if err := s.overrideResponseBody(res, templateInput, cfg.Response.Body); err != nil {
		return fmt.Errorf("error applying body override: %v", err)
	}

	return nil
}

// copyRequest creates a deep copy of an *http.Request, including headers, URL, and body.
func copyRequest(req *http.Request) (*HttpRequest, error) {
	reqCopy := &HttpRequest{
		Method:  req.Method,
		Path:    req.RequestURI,
		Headers: make(http.Header),
	}

	for headerKey, headerValue := range req.Header {
		headerValueCopy := make([]string, len(headerValue))
		copy(headerValueCopy, headerValue)
		reqCopy.Headers[headerKey] = headerValueCopy
	}

	if req.Body != nil {
		bodyBytes, err := copyBody(&req.Body)
		if err != nil {
			return nil, err
		}

		reqCopy.Body = bodyBytes
	}

	return reqCopy, nil
}

// func copyResponse(req *http.Response) (*HttpResponse, error) {
// 	resCopy := &HttpResponse{
// 		Headers: make(http.Header),
// 	}

// 	for headerKey, headerValue := range req.Header {
// 		headerValueCopy := make([]string, len(headerValue))
// 		copy(headerValueCopy, headerValue)
// 		resCopy.Headers[headerKey] = headerValueCopy
// 	}

// 	if req.Body != nil {
// 		bodyBytes, err := copyBody(&req.Body)
// 		if err != nil {
// 			return nil, err
// 		}

// 		resCopy.Body = bodyBytes
// 	}

// 	return resCopy, nil
// }

func copyBody(body *io.ReadCloser) ([]byte, error) {
	// Read the body
	bodyBytes, err := io.ReadAll(*body)
	if err != nil {
		return nil, err
	}

	// Close the original body
	(*body).Close()

	// Replace the body with a new reader containing the same data
	*body = io.NopCloser(bytes.NewReader(bodyBytes))

	return bodyBytes, nil
}

// overrides all the headers and renders the header values at the end to make sure than render functions aren't called
// for routes than they aren't included in. E.g. for headless requests if not done in two steps, the slauth token would
// be requested even though the header gets removed after.
func (s *server) overrideHeaders(headerOperations []config.Header, originalHeaders *http.Header, templateInput map[string]any) error {
	// Track render info for headers that need it
	headerRenderInfo := make(map[string]config.Input)

	for _, headerOperation := range headerOperations {
		if err := s.overrideHeader(headerOperation, originalHeaders, headerRenderInfo); err != nil {
			return err
		}
	}

	for headerKey := range *originalHeaders {
		input, needsRender := headerRenderInfo[strings.ToLower(headerKey)]
		if !needsRender {
			continue
		}

		// Use unified render to support both Template and Expr in header values
		renderedHeaderValueBytes, err := s.renderer.Render(input.Template, input.Expr, templateInput, nil)
		if err != nil {
			return err
		}

		originalHeaders.Del(headerKey)
		originalHeaders.Set(headerKey, string(renderedHeaderValueBytes))
	}

	return nil
}

// overrideHeader modifies the HTTP request headers based on the provided configuration.
// It can remove headers, set header values from text or file, or clear all headers except "Content-Length".
func (s *server) overrideHeader(header config.Header, originalHeaders *http.Header, headerRenderInfo map[string]config.Input) error {
	if header.Operation == config.RemoveOperation {
		if header.Name != "" {
			originalHeaders.Del(header.Name)
			return nil
		}

		// Wipe the headers except for "Content-Length" because it can't be
		// statically set
		for header := range *originalHeaders {
			if header != "Content-Length" {
				originalHeaders.Del(header)
			}
		}

		return nil
	}

	if header.Expr != "" || header.Template != "" {
		// Store the expr/template string as a placeholder (or empty)
		originalHeaders.Set(header.Name, "")

		// Track render info
		headerRenderInfo[strings.ToLower(header.Name)] = config.Input{
			Template: header.Template,
			Expr:     header.Expr,
		}

		return nil
	}

	if header.Text != "" {
		originalHeaders.Set(header.Name, header.Text)
		return nil
	}

	if header.File != "" {
		bytesStr, err := os.ReadFile(header.File)
		if err != nil {
			return err
		}

		originalHeaders.Set(header.Name, string(bytesStr))
		return nil
	}

	body, err := s.makeRequest(header.Request)
	if err != nil {
		return err
	}

	var responseData any

	if err := json.Unmarshal(body, &responseData); err != nil {
		return err
	}

	val, err := s.getValueAtPath(responseData, header.Request.Response.ResultPath)
	if err != nil {
		return err
	}

	originalHeaders.Set(header.Name, val)
	return nil
}
