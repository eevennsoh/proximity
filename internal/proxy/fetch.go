package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"golang.org/x/sync/errgroup"
)

// RequestResult represents the result of a single fetch request
type RequestResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Error  string `json:"error"`
}

// executeFetch executes fetch requests and populates the requests variable in templateInput
func (s *server) executeFetch(ctx context.Context, fetch *config.Fetch, templateInput map[string]any) {
	// Execute all requests in parallel
	requestResults := s.executeFetchRequests(ctx, fetch.Requests, templateInput)

	// Build requests map and add to template input
	requestsMap := make(map[string]any)

	for name, result := range requestResults {
		requestsMap[name] = map[string]any{
			"status": result.Status,
			"body":   result.Body,
			"error":  result.Error,
		}
	}

	templateInput["requests"] = requestsMap
}

// executeFetchRequests runs all requests in parallel
func (s *server) executeFetchRequests(ctx context.Context, requests map[string]config.FetchRequest, templateInput map[string]any) map[string]*RequestResult {
	results := make(map[string]*RequestResult)
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)

	for name, req := range requests {
		g.Go(func() error {
			result := s.executeFetchRequest(ctx, req, templateInput)

			mu.Lock()
			results[name] = result
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		s.Logger.Println(err.Error())
	}

	return results
}

// executeFetchRequest executes a single request with timeout
func (s *server) executeFetchRequest(parentCtx context.Context, req config.FetchRequest, templateInput map[string]any) *RequestResult {
	// Parse timeout (default 30s)
	timeout := 30 * time.Second

	if req.Timeout != "" {
		if t, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = t
		}
	}

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	// Render URL
	urlBytes, err := s.renderer.Render(req.Url.Template, req.Url.Expr, templateInput, nil)
	if err != nil {
		return &RequestResult{Error: fmt.Sprintf("failed to render URL: %v", err)}
	}

	url := req.Url.Text

	if len(urlBytes) > 0 {
		url = strings.TrimSpace(string(urlBytes))
	}

	if url == "" {
		return &RequestResult{Error: "URL is empty"}
	}

	// Build request body
	var bodyReader io.Reader

	if !req.Body.IsEmpty() {
		bodyBytes, err := s.renderer.Render(req.Body.Template, req.Body.Expr, templateInput, nil)
		if err != nil {
			return &RequestResult{Error: fmt.Sprintf("failed to render body: %v", err)}
		}

		if len(bodyBytes) == 0 && req.Body.Text != "" {
			bodyBytes = []byte(req.Body.Text)
		}

		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bodyReader)
	if err != nil {
		return &RequestResult{Error: fmt.Sprintf("failed to create request: %v", err)}
	}

	if err := s.overrideHeaders(req.Headers, &httpReq.Header, templateInput); err != nil {
		return &RequestResult{Error: fmt.Sprintf("failed to render headers: %v", err)}
	}

	// Execute request
	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return &RequestResult{Error: fmt.Sprintf("request failed: %v", err)}
	}
	defer httpResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return &RequestResult{
			Status: httpResp.StatusCode,
			Error:  fmt.Sprintf("failed to read response: %v", err),
		}
	}

	// Set error if non-2xx status
	var errMsg string

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		errMsg = fmt.Sprintf("HTTP %d", httpResp.StatusCode)
	}

	return &RequestResult{
		Status: httpResp.StatusCode,
		Body:   string(respBody),
		Error:  errMsg,
	}
}

// evaluateStatusCode evaluates a StatusCodeInput and returns the resulting status code.
func (s *server) evaluateStatusCode(statusInput config.StatusCodeInput, templateInput map[string]any) int {
	if statusInput.Expr != "" {
		statusBytes, err := s.renderer.Render("", statusInput.Expr, templateInput, nil)

		if err == nil {
			if sc, err := strconv.Atoi(strings.TrimSpace(string(statusBytes))); err == nil {
				return sc
			}
		}

		// Fall through to int or default on error
	}

	if statusInput.Int != 0 {
		return statusInput.Int
	}

	// Default status code
	return 200
}
