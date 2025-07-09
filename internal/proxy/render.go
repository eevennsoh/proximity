package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"
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
func (s *server) renderRequest(originalRequest *http.Request, cfg *endpointProxyConfig) error {
	copiedRequest, err := copyRequest(originalRequest)
	if err != nil {
		return err
	}

	if cfg.Out != "" {
		renderedPath, err := s.renderTemplateString(strings.TrimSpace(cfg.Out), copiedRequest)
		if err != nil {
			return err
		}

		originalRequest.URL.Path = string(renderedPath)
		originalRequest.RequestURI = string(renderedPath)
	}

	for _, headerOperation := range cfg.Request.Headers {
		if err := s.overrideHeader(headerOperation, originalRequest, copiedRequest); err != nil {
			return err
		}
	}

	// Apply body patches/overrides as per config
	if err := s.overrideBody(originalRequest, copiedRequest, cfg.Request.Body); err != nil {
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
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		reqCopy.Body = bodyBytes
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return reqCopy, nil
}

func copyResponse(req *http.Response) (*HttpResponse, error) {
	resCopy := &HttpResponse{
		Headers: make(http.Header),
	}

	for headerKey, headerValue := range req.Header {
		headerValueCopy := make([]string, len(headerValue))
		copy(headerValueCopy, headerValue)
		resCopy.Headers[headerKey] = headerValueCopy
	}

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		resCopy.Body = bodyBytes
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return resCopy, nil
}

// overrideHeader modifies the HTTP request headers based on the provided configuration.
// It can remove headers, set header values from text or file, or clear all headers except "Content-Length".
func (s *server) overrideHeader(header config.Header, originalRequest *http.Request, copiedRequest *HttpRequest) error {
	if header.Operation == config.RemoveOperation {

		if header.Name != "" {
			originalRequest.Header.Del(header.Name)
			return nil
		}

		// Wipe the headers except for "Content-Length" because it can't be
		// statically set
		newHeaders := make(http.Header)
		newHeaders.Add("Content-Length", originalRequest.Header.Get("Content-Length"))
		originalRequest.Header = newHeaders
		return nil
	}

	if header.Text != "" {
		// Render the header value using the original request data to render
		// the header value
		renderedHeaderValueBytes, err := s.renderTemplateString(header.Text, copiedRequest)
		if err != nil {
			return err
		}

		originalRequest.Header.Set(header.Name, string(renderedHeaderValueBytes))
		return nil
	}

	if header.File != "" {
		bytesStr, err := os.ReadFile(header.File)
		if err != nil {
			return err
		}

		originalRequest.Header.Set(header.Name, string(bytesStr))
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

	originalRequest.Header.Set(header.Name, val)
	return nil
}
