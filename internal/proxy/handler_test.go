package proxy

import (
	"bytes"
	"context"
	"io"
	"log"
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
