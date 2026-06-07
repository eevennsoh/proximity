package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccessTokenReturnsStoredTokenWhenFresh verifies unexpired credentials are reused as-is.
func TestAccessTokenReturnsStoredTokenWhenFresh(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, defaultIssuer, nil)

	err := store.Save(Credentials{
		Type:    credentialTypeOauth,
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
	auth := newServiceForTest(store, time.Now(), defaultIssuer, nil)

	_, err := auth.AccessToken(context.Background())

	assert.ErrorContains(t, err, "proximity openai login")
}

// TestAccessTokenRefreshesExpiredCredentials verifies refresh requests and persisted credentials.
func TestAccessTokenRefreshesExpiredCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)

	err := store.Save(Credentials{
		Type:    credentialTypeOauth,
		Access:  "expired-access-token",
		Refresh: "refresh-token",
		Expires: fixedNow.Add(-time.Minute).UnixMilli(),
	})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, oauthTokenPath, request.URL.Path)
		require.NoError(t, request.ParseForm())
		assert.Equal(t, "refresh_token", request.Form.Get("grant_type"))
		assert.Equal(t, defaultClientId, request.Form.Get("client_id"))
		assert.Equal(t, "refresh-token", request.Form.Get("refresh_token"))

		response.Header().Set("Content-Type", "application/json")
		_, err := response.Write([]byte(`{"id_token":"` + unsignedToken(t, map[string]any{
			chatgptAccountIdClaim: "account-from-token",
		}) + `","access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	auth := newServiceForTest(store, fixedNow, server.URL, server.Client())

	got, err := auth.AccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "new-access-token", got)

	credentials, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, Credentials{
		Type:      credentialTypeOauth,
		Refresh:   "new-refresh-token",
		Access:    "new-access-token",
		Expires:   fixedNow.Add(time.Hour).UnixMilli(),
		AccountId: "account-from-token",
	}, credentials)
}

// TestLogoutRemovesCredentials verifies logout deletes stored credentials.
func TestLogoutRemovesCredentials(t *testing.T) {
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, time.Now(), defaultIssuer, nil)

	err := store.Save(Credentials{
		Type:    credentialTypeOauth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)

	err = auth.Logout()

	require.NoError(t, err)
	_, err = store.Load()
	assert.True(t, errors.Is(err, ErrCredentialsNotFound))
}

// TestStatusForMissingCredentials verifies status reports unauthenticated without error.
func TestStatusForMissingCredentials(t *testing.T) {
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, time.Now(), defaultIssuer, nil)

	status, err := auth.Status(context.Background())

	require.NoError(t, err)
	assert.False(t, status.Authenticated)
}

// TestStatusForStoredCredentials verifies status exposes non-sensitive credential state.
func TestStatusForStoredCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, defaultIssuer, nil)
	expiresAt := fixedNow.Add(time.Hour)

	err := store.Save(Credentials{
		Type:      credentialTypeOauth,
		Access:    "access-token",
		Refresh:   "refresh-token",
		Expires:   expiresAt.UnixMilli(),
		AccountId: "account-123",
	})
	require.NoError(t, err)

	status, err := auth.Status(context.Background())

	require.NoError(t, err)
	assert.True(t, status.Authenticated)
	assert.False(t, status.Expired)
	assert.Equal(t, "account-123", status.AccountId)
	assert.True(t, status.ExpiresAt.Equal(expiresAt))
}

// newCredentialStoreForTest returns a temp-file-backed credential store.
func newCredentialStoreForTest(t *testing.T) credentialStore {
	t.Helper()

	return newFileStore(t.TempDir() + "/auth.json")
}

// newServiceForTest returns a service with deterministic test collaborators.
func newServiceForTest(store credentialStore, now time.Time, issuer string, client httpClient) *service {
	if client == nil {
		client = http.DefaultClient
	}

	return &service{
		store:     store,
		client:    client,
		clientId:  defaultClientId,
		issuer:    issuer,
		oauthPort: defaultOauthPort,
		now: func() time.Time {
			return now
		},
		openBrowser: func(string) error {
			return nil
		},
	}
}
