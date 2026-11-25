package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"bitbucket.org/atlassian-developers/proximity/internal/config"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observedProxyRequest struct {
	Host    string
	Headers http.Header
}

func TestProxyReportsDNSFailureToClientAndLog(t *testing.T) {
	for _, protocol := range []string{"", openResponses, openAIResponses, claudeMessages} {
		t.Run(protocol, func(t *testing.T) {
			var logs bytes.Buffer
			s := New(Options{Logger: log.New(&logs, "", 0), Config: &config.Config{}}).(*server)
			target, err := url.Parse("https://gateway.example.test")
			require.NoError(t, err)
			cfg := &endpointProxyConfig{baseEndpoint: target, UriMap: config.UriMap{In: "/test"}, Out: config.OutMethod{Protocol: protocol}}
			proxy := s.endpointProxy(cfg)
			proxy.Transport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Name: "gateway.example.test", Err: "no such host", IsNotFound: true}}
			})
			recorder := httptest.NewRecorder()
			proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/test", nil))

			assert.Equal(t, http.StatusBadGateway, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "DNS")
			assert.Contains(t, recorder.Body.String(), "gateway.example.test")
			assert.Contains(t, logs.String(), "no such host")
			assert.Contains(t, logs.String(), "/test")
			if protocol != "" {
				var body struct {
					Type  string `json:"type"`
					Error struct {
						Type    string `json:"type"`
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
				assert.Equal(t, "api_error", body.Error.Type)
				assert.Contains(t, body.Error.Message, "DNS")
				if protocol == claudeMessages {
					assert.Equal(t, "error", body.Type)
				}
			}
		})
	}
}

func TestProxyPreservesGatewayErrorResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"invalid tenant","requestId":"request-123"}`)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	var logs bytes.Buffer
	s := New(Options{Logger: log.New(&logs, "", 0), Config: &config.Config{}}).(*server)
	cfg := &endpointProxyConfig{baseEndpoint: target, UriMap: config.UriMap{In: "/test"}}
	recorder := httptest.NewRecorder()
	s.endpointProxy(cfg).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/test", nil))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.JSONEq(t, `{"message":"invalid tenant","requestId":"request-123"}`, recorder.Body.String())
	assert.NotContains(t, logs.String(), "proxy error")
}

func TestProxyReportsResponseTransformationFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	var logs bytes.Buffer
	s := New(Options{Logger: log.New(&logs, "", 0), Config: &config.Config{}}).(*server)
	cfg := &endpointProxyConfig{baseEndpoint: target, UriMap: config.UriMap{In: "/test"}, RequestResponse: config.RequestResponse{
		Response: config.OverrideConfig{Body: config.Body{Expr: "missingFunction()"}},
	}}
	recorder := httptest.NewRecorder()
	s.endpointProxy(cfg).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/test", nil))
	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Check Proximity logs")
	assert.NotContains(t, recorder.Body.String(), "missingFunction")
	assert.Contains(t, logs.String(), "missingFunction")
}

// TestHandleEndpointReturnsErrorWhenRequestRenderingFails verifies render errors are not empty 200 responses.
func TestHandleEndpointReturnsErrorWhenRequestRenderingFails(t *testing.T) {
	var logs bytes.Buffer

	serverUnderTest := New(Options{
		Logger: log.New(&logs, "", 0),
		Config: &config.Config{},
		Vars:   make(map[string]any),
	}).(*server)

	target, err := url.Parse("https://api.example.com")
	require.NoError(t, err)

	endpointConfig := &endpointProxyConfig{
		baseEndpoint: target,
		Out: config.OutMethod{
			Method: http.MethodPost,
			Input: config.Input{
				Text: "/v1/messages",
			},
		},
		RequestResponse: config.RequestResponse{
			Request: config.OverrideConfig{
				Headers: []config.Header{
					{
						Operation: config.AddOperation,
						Name:      "Authorization",
						Input: config.Input{
							Expr: "missingFunction()",
						},
					},
				},
			},
		},
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"claude"}`))
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	response := httptest.NewRecorder()

	serverUnderTest.handleEndpoint(endpointConfig).ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Contains(t, response.Body.String(), "failed to render proxy request")
	assert.Contains(t, logs.String(), "missingFunction")
}

// TestEndpointProxyPurgesForwardedHeaders verifies local proxy metadata is not sent upstream.
func TestEndpointProxyPurgesForwardedHeaders(t *testing.T) {
	observedRequests := make(chan observedProxyRequest, 1)
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedRequests <- observedProxyRequest{
			Host:    r.Host,
			Headers: r.Header.Clone(),
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(testServer.Close)

	serverUnderTest := New(Options{
		Logger: log.New(io.Discard, "", 0),
		Config: &config.Config{},
		Vars:   make(map[string]any),
	}).(*server)

	target, err := url.Parse(testServer.URL)
	require.NoError(t, err)

	endpointConfig := &endpointProxyConfig{
		baseEndpoint: target,
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"claude"}`))
	request.Header.Set("Forwarded", "for=127.0.0.1;host=localhost:29573;proto=http")
	request.Header.Set("X-Forwarded-For", "127.0.0.1")
	request.Header.Set("X-Forwarded-Host", "localhost:29573")
	request.Header.Set("X-Forwarded-Proto", "http")
	response := httptest.NewRecorder()

	serverUnderTest.endpointProxy(endpointConfig).ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	observedRequest := <-observedRequests
	assert.Equal(t, target.Host, observedRequest.Host)
	assert.Empty(t, observedRequest.Headers.Values("Forwarded"))
	assert.Empty(t, observedRequest.Headers.Values("X-Forwarded-For"))
	assert.Empty(t, observedRequest.Headers.Values("X-Forwarded-Host"))
	assert.Empty(t, observedRequest.Headers.Values("X-Forwarded-Proto"))
}
