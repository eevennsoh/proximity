package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"bitbucket.org/atlassian-developers/mini-proxy/internal/config"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

type endpointProxyConfig struct {
	baseEndpoint *url.URL

	config.UriMap
	config.OverrideConfig
}

// An endpoint proxy handles proxying a single URI
// The config will have been generated for it be combining the global and uri override configs together
// There is a generic function for an endpoint proxy which takes in config to work
// The config determines how the endpoint proxy operates

func (s *server) endpointProxy(cfg *endpointProxyConfig) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(cfg.baseEndpoint)
	originalDirector := proxy.Director

	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// If an outbound path is defined, override the incoming one
		if cfg.Out != "" {
			req.URL.Path = cfg.Out
			req.RequestURI = req.URL.RequestURI()
		}

		log.Println(req.Method, req.RequestURI, req.Header.Get("User-Agent"))

		for _, header := range cfg.Headers {
			if err := s.overrideHeader(req, header); err != nil {
				log.Fatal(err)
			}
		}

		if err := s.overrideBody(req, cfg.Body); err != nil {
			log.Fatal(err)
		}
	}

	proxy.ModifyResponse = func(res *http.Response) error {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		if len(cfg.Response.Json) == 0 {
			s.applyNewBodyToResponse(res, bodyBytes)
			return nil
		}

		bodyBytes, err = s.applyPatchToJson(cfg.Response.Json, bodyBytes)
		if err != nil {
			return err
		}

		s.applyNewBodyToResponse(res, bodyBytes)
		return nil
	}

	return proxy
}

// overrideHeader modifies the HTTP request headers based on the provided configuration.
// It can remove headers, set header values from text or file, or clear all headers except "Content-Length".
func (s *server) overrideHeader(req *http.Request, header config.Header) error {
	if header.Operation == config.RemoveOperation {
		if header.Name != "" {
			req.Header.Del(header.Name)
			return nil
		}

		// Wipe the headers except for "Content-Length" because it can't be
		// statically set
		newHeaders := http.Header{}
		newHeaders.Add("Content-Length", req.Header.Get("Content-Length"))
		req.Header = newHeaders
		return nil
	}

	if header.Text != "" {
		req.Header.Set(header.Name, header.Text)
		return nil
	}

	if header.File != "" {
		bytesStr, err := os.ReadFile(header.File)
		if err != nil {
			return err
		}

		req.Header.Set(header.Name, string(bytesStr))
		return nil
	}

	body, err := s.makeRequest(header.Request)
	if err != nil {
		return err
	}

	var responseData interface{}

	if err := json.Unmarshal(body, &responseData); err != nil {
		return err
	}

	val, err := s.getValueAtPath(responseData, header.Request.Response.ResultPath)
	if err != nil {
		return err
	}

	req.Header.Set(header.Name, val)
	return nil
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

func (s *server) getValueAtPath(data interface{}, path string) (string, error) {
	pathElements := strings.Split(strings.Trim(path, "/"), "/")

	for _, key := range pathElements {
		switch curr := data.(type) {
		case map[string]interface{}:
			data = curr[key]
		default:
			return "", fmt.Errorf("error parsing response")
		}
	}

	strValue, ok := data.(string)
	if !ok {
		return "", fmt.Errorf("error parsing response, returned type is not a string")
	}

	return strValue, nil
}

func (s *server) overrideBody(req *http.Request, body config.Body) error {
	if body.Text != "" {
		s.applyNewBodyToRequest(req, []byte(body.Text))
		return nil
	}

	if len(body.Json) == 0 {
		return nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	newBody, err := s.applyPatchToJson(body.Json, bodyBytes)
	if err != nil {
		return err
	}

	s.applyNewBodyToRequest(req, newBody)
	return nil
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

func (s *server) applyNewBodyToResponse(res *http.Response, body []byte) {
	res.Body = io.NopCloser(bytes.NewBuffer(body))
	res.ContentLength = int64(len(body))
	res.Header.Set("Content-Length", fmt.Sprint(len(body)))
}
