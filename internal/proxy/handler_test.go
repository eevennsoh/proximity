package proxy

import (
	"bytes"
	"context"
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
