package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthorizationUrlUsesClaudeCompatibleParameters verifies browser OAuth request shape.
func TestAuthorizationUrlUsesClaudeCompatibleParameters(t *testing.T) {
	service := &service{
		clientId:     defaultClientId,
		authorizeUrl: "https://authorize.example/oauth/authorize",
		oauthPort:    3210,
	}

	got, verifier, state, err := service.authorizationUrl()

	require.NoError(t, err)
	assert.Len(t, verifier, pkceVerifierLength)
	assert.Len(t, state, pkceVerifierLength)

	parsed, err := url.Parse(got)
	require.NoError(t, err)

	values := parsed.Query()
	assert.Equal(t, "https://authorize.example", parsed.Scheme+"://"+parsed.Host)
	assert.Equal(t, oauthAuthorizePath, parsed.Path)
	assert.Equal(t, "true", values.Get("code"))
	assert.Equal(t, "code", values.Get("response_type"))
	assert.Equal(t, defaultClientId, values.Get("client_id"))
	assert.Equal(t, "http://localhost:3210/callback", values.Get("redirect_uri"))
	assert.Equal(t, oauthScope, values.Get("scope"))
	assert.Equal(t, "S256", values.Get("code_challenge_method"))
	assert.Equal(t, state, values.Get("state"))
	assert.NotEmpty(t, values.Get("code_challenge"))
}

// TestLoginWithBrowserExchangesCallbackCode verifies the local callback OAuth flow.
func TestLoginWithBrowserExchangesCallbackCode(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	port := freePortForTest(t)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, oauthTokenPath, request.URL.Path)
		assertAnthropicTokenRequest(t, request)

		body := decodeJsonBodyForTest(t, request)
		assert.Equal(t, "authorization_code", body["grant_type"])
		assert.Equal(t, "browser-code", body["code"])
		assert.Equal(t, fmt.Sprintf("http://localhost:%d/callback", port), body["redirect_uri"])
		assert.Equal(t, defaultClientId, body["client_id"])
		assert.Equal(t, body["state"], body["code_verifier"])
		assert.NotEmpty(t, body["code_verifier"])

		writeJsonForTest(t, response, map[string]any{
			"access_token":  "browser-access-token",
			"refresh_token": "browser-refresh-token",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(tokenServer.Close)

	auth := newServiceForTest(store, fixedNow, "https://authorize.example/oauth/authorize", tokenServer.URL+oauthTokenPath, tokenServer.Client())
	auth.oauthPort = port
	auth.openBrowser = func(authorizeUrl string) error {
		parsed, err := url.Parse(authorizeUrl)
		require.NoError(t, err)

		callbackUrl := fmt.Sprintf(
			"http://localhost:%d/callback?code=browser-code&state=%s",
			port,
			url.QueryEscape(parsed.Query().Get("state")),
		)

		response, err := tokenServer.Client().Get(callbackUrl)
		require.NoError(t, err)
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode)

		return nil
	}

	var output bytes.Buffer

	err := auth.LoginWithBrowser(context.Background(), &output)

	require.NoError(t, err)

	credentials, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Refresh: "browser-refresh-token",
		Access:  "browser-access-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	}, credentials)
	assert.Contains(t, output.String(), "opened browser")
}

// assertAnthropicTokenRequest verifies headers required by the Claude OAuth endpoint.
func assertAnthropicTokenRequest(t *testing.T, request *http.Request) {
	t.Helper()

	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
	assert.Equal(t, anthropicUserAgent, request.Header.Get("User-Agent"))
	assert.Equal(t, anthropicAppHeader, request.Header.Get("X-App"))
}

// decodeJsonBodyForTest decodes a JSON request body into a map.
func decodeJsonBodyForTest(t *testing.T, request *http.Request) map[string]string {
	t.Helper()

	var body map[string]string

	err := json.NewDecoder(request.Body).Decode(&body)
	require.NoError(t, err)

	return body
}

// freePortForTest returns an available localhost TCP port.
func freePortForTest(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	return address.Port
}

// writeJsonForTest writes a JSON response for OAuth test servers.
func writeJsonForTest(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(response).Encode(value)
	require.NoError(t, err)
}
