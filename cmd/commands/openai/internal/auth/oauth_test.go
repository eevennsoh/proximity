package auth

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthorizationUrlUsesOpenCodeCompatibleParameters verifies browser OAuth request shape.
func TestAuthorizationUrlUsesOpenCodeCompatibleParameters(t *testing.T) {
	service := &service{
		clientId:  defaultClientId,
		issuer:    "https://issuer.example",
		oauthPort: 3210,
	}

	got, verifier, state, err := service.authorizationUrl()

	require.NoError(t, err)
	assert.Len(t, verifier, pkceVerifierLength)
	assert.Len(t, state, pkceVerifierLength)

	parsed, err := url.Parse(got)
	require.NoError(t, err)

	values := parsed.Query()
	assert.Equal(t, "https://issuer.example", parsed.Scheme+"://"+parsed.Host)
	assert.Equal(t, oauthAuthorizePath, parsed.Path)
	assert.Equal(t, "code", values.Get("response_type"))
	assert.Equal(t, defaultClientId, values.Get("client_id"))
	assert.Equal(t, "http://localhost:3210/auth/callback", values.Get("redirect_uri"))
	assert.Equal(t, oauthScope, values.Get("scope"))
	assert.Equal(t, "S256", values.Get("code_challenge_method"))
	assert.Equal(t, "true", values.Get("id_token_add_organizations"))
	assert.Equal(t, "true", values.Get("codex_cli_simplified_flow"))
	assert.Equal(t, "opencode", values.Get("originator"))
	assert.Equal(t, state, values.Get("state"))
	assert.NotEmpty(t, values.Get("code_challenge"))
}

// TestLoginWithBrowserExchangesCallbackCode verifies the local callback OAuth flow.
func TestLoginWithBrowserExchangesCallbackCode(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	port := freePortForTest(t)

	issuer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, oauthTokenPath, request.URL.Path)
		require.NoError(t, request.ParseForm())
		assert.Equal(t, "authorization_code", request.Form.Get("grant_type"))
		assert.Equal(t, "browser-code", request.Form.Get("code"))
		assert.Equal(t, fmt.Sprintf("http://localhost:%d/auth/callback", port), request.Form.Get("redirect_uri"))
		assert.Equal(t, defaultClientId, request.Form.Get("client_id"))
		assert.NotEmpty(t, request.Form.Get("code_verifier"))

		response.Header().Set("Content-Type", "application/json")
		_, err := response.Write([]byte(`{"id_token":"` + unsignedToken(t, map[string]any{
			chatgptAccountIdClaim: "account-from-browser",
		}) + `","access_token":"browser-access-token","refresh_token":"browser-refresh-token","expires_in":3600}`))
		require.NoError(t, err)
	}))
	t.Cleanup(issuer.Close)

	auth := newServiceForTest(store, fixedNow, issuer.URL, issuer.Client())
	auth.oauthPort = port
	auth.openBrowser = func(authorizeUrl string) error {
		parsed, err := url.Parse(authorizeUrl)
		require.NoError(t, err)

		callbackUrl := fmt.Sprintf(
			"http://localhost:%d/auth/callback?code=browser-code&state=%s",
			port,
			url.QueryEscape(parsed.Query().Get("state")),
		)

		response, err := issuer.Client().Get(callbackUrl)
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
	assert.Equal(t, Credentials{
		Type:      credentialTypeOauth,
		Refresh:   "browser-refresh-token",
		Access:    "browser-access-token",
		Expires:   fixedNow.Add(time.Hour).UnixMilli(),
		AccountId: "account-from-browser",
	}, credentials)
	assert.Contains(t, output.String(), "opened browser")
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
