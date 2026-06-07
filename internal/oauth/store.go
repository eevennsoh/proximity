package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	credentialsFileMode = 0600
	credentialsDirMode  = 0700
	configDirectoryName = "proximity"
	credentialsFileName = "auth.json"
)

var (
	// ErrCredentialsNotFound reports that no stored OAuth credentials exist.
	ErrCredentialsNotFound = errors.New("oauth credentials not found")
)

type fileStore struct {
	path string
}

// NewFileStore returns a credential store backed by a JSON file.
func NewFileStore(path string) Store {
	return &fileStore{
		path: path,
	}
}

// DefaultCredentialPath returns the default credential file path for a provider namespace.
func DefaultCredentialPath(provider string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user config directory: %w", err)
	}

	return filepath.Join(configDir, configDirectoryName, provider, credentialsFileName), nil
}

// Load reads credentials from disk.
func (s *fileStore) Load() (Credentials, error) {
	bytes, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrCredentialsNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("failed to read oauth credentials: %w", err)
	}

	var credentials Credentials

	if err := json.Unmarshal(bytes, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("failed to parse oauth credentials: %w", err)
	}

	return credentials, nil
}

// Save writes credentials to disk with user-only permissions.
func (s *fileStore) Save(credentials Credentials) error {
	if err := os.MkdirAll(filepath.Dir(s.path), credentialsDirMode); err != nil {
		return fmt.Errorf("failed to create oauth credential directory: %w", err)
	}

	bytes, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode oauth credentials: %w", err)
	}

	if err := os.WriteFile(s.path, bytes, credentialsFileMode); err != nil {
		return fmt.Errorf("failed to write oauth credentials: %w", err)
	}

	return setCredentialFileMode(s.path)
}

// setCredentialFileMode applies user-only permissions to the credential file.
func setCredentialFileMode(path string) error {
	if err := os.Chmod(path, credentialsFileMode); err != nil {
		return fmt.Errorf("failed to set oauth credential permissions: %w", err)
	}

	return nil
}

// Remove deletes the credential file when it exists.
func (s *fileStore) Remove() error {
	if err := os.Remove(s.path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to remove oauth credentials: %w", err)
	}

	return nil
}
