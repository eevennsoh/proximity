package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccessTokenReturnsStoredTokenWhenFresh verifies unexpired credentials are reused as-is.
func TestAccessTokenReturnsStoredTokenWhenFresh(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, defaultIssuer, nil)

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
	auth := newServiceForTest(store, time.Now(), defaultIssuer, nil)

	_, err := auth.AccessToken(context.Background())

	assert.ErrorContains(t, err, "proximity openai login")
}

// TestAccessTokenRefreshesExpiredCredentials verifies refresh requests and persisted credentials.
func TestAccessTokenRefreshesExpiredCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 6, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)

	err := store.Save(internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
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
	assert.Equal(t, openaiCredentialsForTest("new-access-token", "new-refresh-token", fixedNow.Add(time.Hour), "account-from-token"), credentials)
}

// TestLogoutRemovesCredentials verifies logout deletes stored credentials.
func TestLogoutRemovesCredentials(t *testing.T) {
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, time.Now(), defaultIssuer, nil)

	err := store.Save(internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)

	err = auth.Logout()

	require.NoError(t, err)
	_, err = store.Load()
	assert.True(t, errors.Is(err, internaloauth.ErrCredentialsNotFound))
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

	err := store.Save(openaiCredentialsForTest("access-token", "refresh-token", expiresAt, "account-123"))
	require.NoError(t, err)

	status, err := auth.Status(context.Background())

	require.NoError(t, err)
	assert.True(t, status.Authenticated)
	assert.False(t, status.Expired)
	assert.Equal(t, "account-123", status.AccountId)
	assert.True(t, status.ExpiresAt.Equal(expiresAt))
}

// newCredentialStoreForTest returns a temp-file-backed credential store.
func newCredentialStoreForTest(t *testing.T) internaloauth.Store {
	t.Helper()

	return internaloauth.NewFileStore(t.TempDir() + "/auth.json")
}

// newServiceForTest returns a service with deterministic test collaborators.
func newServiceForTest(store internaloauth.Store, now time.Time, issuer string, client httpClient) *service {
	if client == nil {
		client = http.DefaultClient
	}

	tokens := &tokenClient{
		client:   client,
		clientId: defaultClientId,
		issuer:   issuer,
		now: func() time.Time {
			return now
		},
	}

	return &service{
		store: store,
		oauthService: internaloauth.New(store, tokens, internaloauth.WithNow(func() time.Time {
			return now
		})),
		tokens:    tokens,
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

// openaiCredentialsForTest returns generic OAuth credentials with OpenAI account metadata.
func openaiCredentialsForTest(access string, refresh string, expires time.Time, accountId string) internaloauth.Credentials {
	credentials := internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Access:  access,
		Refresh: refresh,
		Expires: expires.UnixMilli(),
	}

	if accountId != "" {
		credentials.Metadata = make(map[string]string)
		credentials.Metadata[accountIdMetadataKey] = accountId
	}

	return credentials
}
