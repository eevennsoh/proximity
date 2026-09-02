package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccessTokenReturnsStoredTokenWhenFresh verifies unexpired credentials are reused as-is.
func TestAccessTokenReturnsStoredTokenWhenFresh(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, "https://device.example", "https://token.example", nil)

	err := store.Save(internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)

	got, err := auth.AccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "access-token", got)
}

// TestAccessTokenMissingCredentialsReturnsLoginError verifies missing credentials guide login.
func TestAccessTokenMissingCredentialsReturnsLoginError(t *testing.T) {
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, time.Now(), "https://device.example", "https://token.example", nil)

	_, err := auth.AccessToken(context.Background())

	assert.ErrorContains(t, err, "proximity rovo login")
}

// TestAccessTokenRefreshesExpiredCredentials verifies the refresh grant and persisted credentials.
func TestAccessTokenRefreshesExpiredCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)

	err := store.Save(internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Access:  "expired-access-token",
		Refresh: "refresh-token",
		Expires: fixedNow.Add(-time.Minute).UnixMilli(),
	})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assertFormTokenRequest(t, request)

		form := decodeFormBodyForTest(t, request)
		assert.Equal(t, grantTypeRefreshToken, form.Get("grant_type"))
		assert.Equal(t, defaultProdClientId, form.Get("client_id"))
		assert.Equal(t, "refresh-token", form.Get("refresh_token"))

		writeJsonForTest(t, response, http.StatusOK, `{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600}`)
	}))
	t.Cleanup(server.Close)

	auth := newServiceForTest(store, fixedNow, "https://device.example", server.URL, server.Client())

	got, err := auth.AccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "new-access-token", got)

	credentials, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Refresh: "new-refresh-token",
		Access:  "new-access-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	}, credentials)
}

// TestLoginWithDevicePollsUntilApproval verifies the device flow initiation, polling, and storage.
func TestLoginWithDevicePollsUntilApproval(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)

	var pollCount atomic.Int32

	deviceServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assertFormTokenRequest(t, request)
		form := decodeFormBodyForTest(t, request)

		if request.URL.Path == "/device" {
			assert.Equal(t, defaultProdClientId, form.Get("client_id"))
			writeJsonForTest(t, response, http.StatusOK, `{"device_code":"device-abc","user_code":"WXYZ-1234","verification_uri":"https://id.atlassian.com/device","verification_uri_complete":"https://id.atlassian.com/device?user_code=WXYZ-1234","expires_in":600,"interval":1}`)
			return
		}

		assert.Equal(t, grantTypeDeviceCode, form.Get("grant_type"))
		assert.Equal(t, "device-abc", form.Get("device_code"))
		assert.Equal(t, defaultProdClientId, form.Get("client_id"))

		if pollCount.Add(1) == 1 {
			writeJsonForTest(t, response, http.StatusBadRequest, `{"error":"authorization_pending"}`)
			return
		}

		writeJsonForTest(t, response, http.StatusOK, `{"access_token":"device-access-token","refresh_token":"device-refresh-token","expires_in":3600}`)
	}))
	t.Cleanup(deviceServer.Close)

	auth := newServiceForTest(store, fixedNow, deviceServer.URL+"/device", deviceServer.URL+"/token", deviceServer.Client())

	var openedUrl string
	auth.openBrowser = func(browserUrl string) error {
		openedUrl = browserUrl
		return nil
	}

	err := auth.LoginWithDevice(context.Background(), io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "https://id.atlassian.com/device?user_code=WXYZ-1234", openedUrl)
	assert.GreaterOrEqual(t, pollCount.Load(), int32(2))

	credentials, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Refresh: "device-refresh-token",
		Access:  "device-access-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	}, credentials)
}

// TestLoginWithDeviceFailsWhenAccessDenied verifies denial short-circuits without storing credentials.
func TestLoginWithDeviceFailsWhenAccessDenied(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/device" {
			writeJsonForTest(t, response, http.StatusOK, `{"device_code":"device-abc","user_code":"WXYZ-1234","verification_uri":"https://id.atlassian.com/device","expires_in":600,"interval":1}`)
			return
		}

		writeJsonForTest(t, response, http.StatusForbidden, `{"error":"access_denied","error_description":"user rejected"}`)
	}))
	t.Cleanup(deviceServer.Close)

	auth := newServiceForTest(store, fixedNow, deviceServer.URL+"/device", deviceServer.URL+"/token", deviceServer.Client())
	auth.openBrowser = func(string) error {
		return nil
	}

	err := auth.LoginWithDevice(context.Background(), io.Discard)

	assert.ErrorContains(t, err, "denied")
	assert.ErrorContains(t, err, "user rejected")

	_, loadErr := store.Load()
	assert.ErrorIs(t, loadErr, internaloauth.ErrCredentialsNotFound)
}

// TestStatusReportsExpiry verifies stored credential state is surfaced without token values.
func TestStatusReportsExpiry(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, "https://device.example", "https://token.example", nil)

	expiresAt := fixedNow.Add(time.Hour)

	err := store.Save(internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: expiresAt.UnixMilli(),
	})
	require.NoError(t, err)

	got, err := auth.Status(context.Background())

	require.NoError(t, err)
	assert.True(t, got.Authenticated)
	assert.False(t, got.Expired)
	assert.Equal(t, expiresAt.UnixMilli(), got.ExpiresAt.UnixMilli())
}

// newCredentialStoreForTest returns a file-backed store in a temp directory.
func newCredentialStoreForTest(t *testing.T) internaloauth.Store {
	t.Helper()

	return internaloauth.NewFileStore(t.TempDir() + "/auth.json")
}

// newServiceForTest builds a service wired to test endpoints and a fixed clock.
func newServiceForTest(store internaloauth.Store, now time.Time, deviceCodeUrl string, tokenUrl string, client httpClient) *service {
	if client == nil {
		client = http.DefaultClient
	}

	clock := func() time.Time {
		return now
	}

	tokens := &tokenClient{
		client:   client,
		clientId: defaultProdClientId,
		tokenUrl: tokenUrl,
		now:      clock,
	}

	return &service{
		store:         store,
		oauthService:  internaloauth.New(store, tokens, internaloauth.WithNow(clock)),
		tokens:        tokens,
		clientId:      defaultProdClientId,
		deviceCodeUrl: deviceCodeUrl,
		now:           clock,
		openBrowser: func(string) error {
			return nil
		},
	}
}

// assertFormTokenRequest verifies headers required by the Atlassian OAuth endpoints.
func assertFormTokenRequest(t *testing.T, request *http.Request) {
	t.Helper()

	assert.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", request.Header.Get("Accept"))
}

// decodeFormBodyForTest decodes a form-encoded request body.
func decodeFormBodyForTest(t *testing.T, request *http.Request) url.Values {
	t.Helper()

	body, err := io.ReadAll(request.Body)
	require.NoError(t, err)

	values, err := url.ParseQuery(string(body))
	require.NoError(t, err)

	return values
}

// writeJsonForTest writes a JSON response with the given status code.
func writeJsonForTest(t *testing.T, response http.ResponseWriter, status int, body string) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)

	_, err := io.WriteString(response, body)
	require.NoError(t, err)
}
