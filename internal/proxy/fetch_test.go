package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/template"
)

func newTestServer() *server {
	return &server{
		renderer: template.NewRenderer(nil),
	}
}

func TestExecuteFetchRequest(t *testing.T) {
	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "hello"}`))
	}))
	defer testSrv.Close()

	s := newTestServer()

	req := config.FetchRequest{
		Method: "GET",
		Url: config.Input{
			Text: testSrv.URL,
		},
	}

	templateInput := map[string]any{}
	result := s.executeFetchRequest(context.Background(), req, templateInput)

	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if result.Status != 200 {
		t.Errorf("Expected status 200, got: %d", result.Status)
	}
	if result.Body != `{"message": "hello"}` {
		t.Errorf("Unexpected body: %s", result.Body)
	}
}

func TestExecuteFetchRequestWithExprUrl(t *testing.T) {
	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`ok`))
	}))
	defer testSrv.Close()

	s := newTestServer()

	req := config.FetchRequest{
		Method: "GET",
		Url: config.Input{
			Expr: `settings.baseUrl`,
		},
	}

	templateInput := map[string]any{
		"settings": map[string]any{
			"baseUrl": testSrv.URL,
		},
	}

	result := s.executeFetchRequest(context.Background(), req, templateInput)

	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if result.Status != 200 {
		t.Errorf("Expected status 200, got: %d", result.Status)
	}
}

func TestExecuteFetchRequestTimeout(t *testing.T) {
	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer testSrv.Close()

	s := newTestServer()

	req := config.FetchRequest{
		Method:  "GET",
		Url:     config.Input{Text: testSrv.URL},
		Timeout: "50ms",
	}

	result := s.executeFetchRequest(context.Background(), req, map[string]any{})

	if result.Error == "" {
		t.Error("Expected timeout error, got none")
	}
}

func TestExecuteFetchRequestNon2xx(t *testing.T) {
	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}))
	defer testSrv.Close()

	s := newTestServer()

	req := config.FetchRequest{
		Method: "GET",
		Url:    config.Input{Text: testSrv.URL},
	}

	result := s.executeFetchRequest(context.Background(), req, map[string]any{})

	if result.Status != 404 {
		t.Errorf("Expected status 404, got: %d", result.Status)
	}
	if result.Error != "HTTP 404" {
		t.Errorf("Expected error 'HTTP 404', got: %s", result.Error)
	}
	if result.Body != `{"error": "not found"}` {
		t.Errorf("Unexpected body: %s", result.Body)
	}
}

func TestExecuteFetchRequests(t *testing.T) {
	testSrv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 1}`))
	}))
	defer testSrv1.Close()

	testSrv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 2}`))
	}))
	defer testSrv2.Close()

	s := newTestServer()

	requests := map[string]config.FetchRequest{
		"first": {
			Method: "GET",
			Url:    config.Input{Text: testSrv1.URL},
		},
		"second": {
			Method: "GET",
			Url:    config.Input{Text: testSrv2.URL},
		},
	}

	results := s.executeFetchRequests(context.Background(), requests, map[string]any{})

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got: %d", len(results))
	}

	if results["first"].Body != `{"id": 1}` {
		t.Errorf("Unexpected first body: %s", results["first"].Body)
	}
	if results["second"].Body != `{"id": 2}` {
		t.Errorf("Unexpected second body: %s", results["second"].Body)
	}
}

func TestEvaluateStatusCode(t *testing.T) {
	s := newTestServer()

	tests := []struct {
		name     string
		input    config.StatusCodeInput
		template map[string]any
		expected int
	}{
		{
			name:     "static int",
			input:    config.StatusCodeInput{Int: 201},
			template: map[string]any{},
			expected: 201,
		},
		{
			name:     "expr success",
			input:    config.StatusCodeInput{Expr: `requests.test.error == "" ? 200 : 502`},
			template: map[string]any{"requests": map[string]any{"test": map[string]any{"error": ""}}},
			expected: 200,
		},
		{
			name:     "expr error",
			input:    config.StatusCodeInput{Expr: `requests.test.error == "" ? 200 : 502`},
			template: map[string]any{"requests": map[string]any{"test": map[string]any{"error": "HTTP 500"}}},
			expected: 502,
		},
		{
			name:     "default to 200",
			input:    config.StatusCodeInput{},
			template: map[string]any{},
			expected: 200,
		},
		{
			name:     "expr takes precedence over int",
			input:    config.StatusCodeInput{Int: 201, Expr: "404"},
			template: map[string]any{},
			expected: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.evaluateStatusCode(tt.input, tt.template)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestExecuteFetch(t *testing.T) {
	testSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": "test"}`))
	}))
	defer testSrv.Close()

	s := newTestServer()

	fetch := &config.Fetch{
		Requests: map[string]config.FetchRequest{
			"api": {
				Method: "GET",
				Url:    config.Input{Text: testSrv.URL},
			},
		},
	}

	templateInput := map[string]any{}
	s.executeFetch(context.Background(), fetch, templateInput)

	requests, ok := templateInput["requests"].(map[string]any)
	if !ok {
		t.Fatal("requests not found in templateInput")
	}

	api, ok := requests["api"].(map[string]any)
	if !ok {
		t.Fatal("api not found in requests")
	}

	if api["status"] != 200 {
		t.Errorf("Expected status 200, got: %v", api["status"])
	}
	if api["body"] != `{"data": "test"}` {
		t.Errorf("Unexpected body: %v", api["body"])
	}
	if api["error"] != "" {
		t.Errorf("Expected no error, got: %v", api["error"])
	}
}
