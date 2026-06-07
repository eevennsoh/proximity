package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoginWithDevicePollsAndStoresCredentials verifies the headless device OAuth flow.
func TestLoginWithDevicePollsAndStoresCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	polls := 0

	issuer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			assertDeviceUserCodeRequest(t, request)
			writeJsonForTest(t, response, map[string]string{
				"device_auth_id": "device-123",
				"user_code":      "ABCD-EFGH",
				"interval":       "0",
			})
		case "/api/accounts/deviceauth/token":
			polls++
			assertDeviceTokenPollRequest(t, request)
			if polls == 1 {
				response.WriteHeader(http.StatusForbidden)
				return
			}

			writeJsonForTest(t, response, map[string]string{
				"authorization_code": "auth-code",
				"code_verifier":      "verifier",
			})
		case oauthTokenPath:
			require.NoError(t, request.ParseForm())
			assert.Equal(t, "authorization_code", request.Form.Get("grant_type"))
			assert.Equal(t, "https://auth.openai.com/deviceauth/callback", request.Form.Get("redirect_uri"))
			assert.Equal(t, defaultClientId, request.Form.Get("client_id"))
			assert.Equal(t, "auth-code", request.Form.Get("code"))
			assert.Equal(t, "verifier", request.Form.Get("code_verifier"))

			writeJsonForTest(t, response, map[string]any{
				"id_token":      unsignedToken(t, map[string]any{chatgptAccountIdClaim: "account-from-device"}),
				"access_token":  "device-access-token",
				"refresh_token": "device-refresh-token",
				"expires_in":    3600,
			})
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(issuer.Close)

	auth := newServiceForTest(store, fixedNow, issuer.URL, issuer.Client())
	var output bytes.Buffer

	err := auth.LoginWithDevice(context.Background(), &output)

	require.NoError(t, err)
	assert.Equal(t, 2, polls)
	assert.Contains(t, output.String(), "https://auth.openai.com/codex/device")
	assert.Contains(t, output.String(), "ABCD-EFGH")

	credentials, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, Credentials{
		Type:      credentialTypeOauth,
		Refresh:   "device-refresh-token",
		Access:    "device-access-token",
		Expires:   fixedNow.Add(time.Hour).UnixMilli(),
		AccountId: "account-from-device",
	}, credentials)
}

// assertDeviceUserCodeRequest verifies the device user-code request body.
func assertDeviceUserCodeRequest(t *testing.T, request *http.Request) {
	t.Helper()

	require.Equal(t, http.MethodPost, request.Method)

	var body map[string]string

	err := json.NewDecoder(request.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, defaultClientId, body["client_id"])
}

// assertDeviceTokenPollRequest verifies the device token polling request body.
func assertDeviceTokenPollRequest(t *testing.T, request *http.Request) {
	t.Helper()

	require.Equal(t, http.MethodPost, request.Method)

	var body map[string]string

	err := json.NewDecoder(request.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "device-123", body["device_auth_id"])
	assert.Equal(t, "ABCD-EFGH", body["user_code"])
}

// writeJsonForTest writes a JSON response for OAuth test servers.
func writeJsonForTest(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(response).Encode(value)
	require.NoError(t, err)
}
