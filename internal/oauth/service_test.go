package oauth

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestAccessTokenReturnsStoredTokenWhenFresh verifies unexpired credentials are reused as-is.
func TestAccessTokenReturnsStoredTokenWhenFresh(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newFileStoreForTest(t)
	refresher := NewMockRefresher(gomock.NewController(t))
	refresher.EXPECT().Refresh(gomock.Any(), gomock.Any()).Times(0)

	err := store.Save(Credentials{
		Type:    CredentialTypeOauth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	})
	require.NoError(t, err)

	service := New(store, refresher, WithNow(func() time.Time {
		return fixedNow
	}))

	got, err := service.AccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "access-token", got)
}

// TestAccessTokenReturnsMissingCredentialsError verifies missing credentials are generic.
func TestAccessTokenReturnsMissingCredentialsError(t *testing.T) {
	store := newFileStoreForTest(t)
	refresher := NewMockRefresher(gomock.NewController(t))
	refresher.EXPECT().Refresh(gomock.Any(), gomock.Any()).Times(0)
	service := New(store, refresher)

	_, err := service.AccessToken(context.Background())

	assert.True(t, errors.Is(err, ErrCredentialsNotFound))
}

// TestAccessTokenRefreshesExpiredCredentials verifies refresh requests and persistence.
func TestAccessTokenRefreshesExpiredCredentials(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newFileStoreForTest(t)
	expired := Credentials{
		Type:    CredentialTypeOauth,
		Access:  "expired-access-token",
		Refresh: "refresh-token",
		Expires: fixedNow.Add(-time.Minute).UnixMilli(),
	}
	updated := Credentials{
		Type:    CredentialTypeOauth,
		Access:  "new-access-token",
		Refresh: "new-refresh-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	}

	err := store.Save(expired)
	require.NoError(t, err)

	refresher := NewMockRefresher(gomock.NewController(t))
	refresher.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, credentials Credentials) (Credentials, error) {
			assert.Equal(t, expired, credentials)
			return updated, nil
		},
	).Times(1)
	service := New(store, refresher, WithNow(func() time.Time {
		return fixedNow
	}))

	got, err := service.AccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "new-access-token", got)

	credentials, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, updated, credentials)
}

// TestAccessTokenCoalescesConcurrentRefreshes verifies only one refresh runs for a stale token.
func TestAccessTokenCoalescesConcurrentRefreshes(t *testing.T) {
	fixedNow := time.Date(2026, time.June, 7, 12, 0, 0, 0, time.UTC)
	store := newFileStoreForTest(t)
	expired := Credentials{
		Type:    CredentialTypeOauth,
		Access:  "expired-access-token",
		Refresh: "refresh-token",
		Expires: fixedNow.Add(-time.Minute).UnixMilli(),
	}
	updated := Credentials{
		Type:    CredentialTypeOauth,
		Access:  "new-access-token",
		Refresh: "new-refresh-token",
		Expires: fixedNow.Add(time.Hour).UnixMilli(),
	}

	err := store.Save(expired)
	require.NoError(t, err)

	refresher := NewMockRefresher(gomock.NewController(t))
	refresher.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return(updated, nil).Times(1)
	service := New(store, refresher, WithNow(func() time.Time {
		return fixedNow
	}))

	var waitGroup sync.WaitGroup
	errors := make(chan error, 2)
	tokens := make(chan string, 2)

	for range 2 {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			token, err := service.AccessToken(context.Background())
			errors <- err
			tokens <- token
		}()
	}

	waitGroup.Wait()
	close(errors)
	close(tokens)

	for err := range errors {
		require.NoError(t, err)
	}

	for token := range tokens {
		assert.Equal(t, "new-access-token", token)
	}
}

// newFileStoreForTest returns a temp-file-backed credential store.
func newFileStoreForTest(t *testing.T) Store {
	t.Helper()

	return NewFileStore(filepath.Join(t.TempDir(), "auth.json"))
}
