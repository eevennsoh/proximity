package oauth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileStoreSaveLoadAndRemove verifies credential persistence, permissions, and deletion.
func TestFileStoreSaveLoadAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proximity", "oauth", "auth.json")
	store := NewFileStore(path)
	expires := time.Now().Add(time.Hour)
	metadata := make(map[string]string)
	metadata["accountId"] = "account-123"

	credentials := Credentials{
		Type:     CredentialTypeOauth,
		Refresh:  "refresh-token",
		Access:   "access-token",
		Expires:  expires.UnixMilli(),
		Metadata: metadata,
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
	store := NewFileStore(filepath.Join(t.TempDir(), "auth.json"))

	_, err := store.Load()

	assert.True(t, errors.Is(err, ErrCredentialsNotFound))
}

// TestDefaultCredentialPathUsesProviderNamespace verifies default paths are provider-scoped.
func TestDefaultCredentialPathUsesProviderNamespace(t *testing.T) {
	path, err := DefaultCredentialPath("anthropic")

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, filepath.Join("proximity", "anthropic", "auth.json")))
}

// TestSetCredentialFileModeWrapsErrors verifies permission failures are contextual.
func TestSetCredentialFileModeWrapsErrors(t *testing.T) {
	err := setCredentialFileMode(filepath.Join(t.TempDir(), "missing.json"))

	assert.ErrorContains(t, err, "failed to set oauth credential permissions")
}
