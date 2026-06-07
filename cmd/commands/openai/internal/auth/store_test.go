package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileStoreSaveLoadAndRemove verifies credential persistence, permissions, and deletion.
func TestFileStoreSaveLoadAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proximity", "openai", "auth.json")
	store := newFileStore(path)
	expires := time.Now().Add(time.Hour)

	credentials := Credentials{
		Type:      credentialTypeOauth,
		Refresh:   "refresh-token",
		Access:    "access-token",
		Expires:   expires.UnixMilli(),
		AccountId: "account-123",
	}

	err := store.Save(credentials)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, credentials, loaded)

	err = store.Remove()
	require.NoError(t, err)

	_, err = store.Load()
	assert.True(t, errors.Is(err, ErrCredentialsNotFound))
}

// TestFileStoreLoadMissingFile verifies absent credentials return the sentinel error.
func TestFileStoreLoadMissingFile(t *testing.T) {
	store := newFileStore(filepath.Join(t.TempDir(), "auth.json"))

	_, err := store.Load()

	assert.True(t, errors.Is(err, ErrCredentialsNotFound))
}
