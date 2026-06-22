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
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, "https://auth.example", "https://token.example", nil)

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
	auth := newServiceForTest(store, time.Now(), "https://auth.example", "https://token.example", nil)

	_, err := auth.AccessToken(context.Background())

	assert.ErrorContains(t, err, "proximity anthropic login")
}

// TestAccessTokenRefreshesExpiredCredentials verifies refresh requests and persisted credentials.
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
		require.Equal(t, oauthTokenPath, request.URL.Path)
		assertAnthropicTokenRequest(t, request)

		body := decodeJsonBodyForTest(t, request)
		assert.Equal(t, "refresh_token", body["grant_type"])
		assert.Equal(t, defaultClientId, body["client_id"])
		assert.Equal(t, "refresh-token", body["refresh_token"])

		writeJsonForTest(t, response, map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	auth := newServiceForTest(store, fixedNow, "https://auth.example", server.URL+oauthTokenPath, server.Client())

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

// TestLogoutRemovesCredentials verifies logout deletes stored credentials.
func TestLogoutRemovesCredentials(t *testing.T) {
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, time.Now(), "https://auth.example", "https://token.example", nil)

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

// TestStatusForStoredCredentials verifies status exposes non-sensitive credential state.
func TestStatusForStoredCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newCredentialStoreForTest(t)
	auth := newServiceForTest(store, fixedNow, "https://auth.example", "https://token.example", nil)
	expiresAt := fixedNow.Add(time.Hour)

	err := store.Save(internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: expiresAt.UnixMilli(),
	})
	require.NoError(t, err)

	status, err := auth.Status(context.Background())

	require.NoError(t, err)
	assert.True(t, status.Authenticated)
	assert.False(t, status.Expired)
	assert.True(t, status.ExpiresAt.Equal(expiresAt))
}

// newCredentialStoreForTest returns a temp-file-backed credential store.
func newCredentialStoreForTest(t *testing.T) internaloauth.Store {
	t.Helper()

	return internaloauth.NewFileStore(t.TempDir() + "/auth.json")
}

// newServiceForTest returns a service with deterministic test collaborators.
func newServiceForTest(store internaloauth.Store, now time.Time, authorizeUrl string, tokenUrl string, client httpClient) *service {
	if client == nil {
		client = http.DefaultClient
	}

	tokens := &tokenClient{
		client:   client,
		clientId: defaultClientId,
		tokenUrl: tokenUrl,
		now: func() time.Time {
			return now
		},
	}

	return &service{
		store: store,
		oauthService: internaloauth.New(store, tokens, internaloauth.WithNow(func() time.Time {
			return now
		})),
		tokens:       tokens,
		clientId:     defaultClientId,
		authorizeUrl: authorizeUrl,
		oauthPort:    defaultOauthPort,
		now: func() time.Time {
			return now
		},
		openBrowser: func(string) error {
			return nil
		},
	}
}
