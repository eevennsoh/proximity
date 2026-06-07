package auth

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
)

var (
	// ErrCredentialsNotFound reports that no stored OpenAI credentials exist.
	ErrCredentialsNotFound = errors.New("openai credentials not found")
)

type credentialStore interface {
	Load() (Credentials, error)
	Save(credentials Credentials) error
	Remove() error
}

type fileStore struct {
	path string
}

// newFileStore returns a credential store backed by a JSON file.
func newFileStore(path string) credentialStore {
	return &fileStore{
		path: path,
	}
}

// defaultCredentialPath returns Proximity's OpenAI credential file path.
func defaultCredentialPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user config directory: %w", err)
	}

	return filepath.Join(configDir, "proximity", "openai", "auth.json"), nil
}

// Load reads credentials from disk.
func (s *fileStore) Load() (Credentials, error) {
	bytes, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrCredentialsNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("failed to read openai credentials: %w", err)
	}

	var credentials Credentials

	if err := json.Unmarshal(bytes, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("failed to parse openai credentials: %w", err)
	}

	return credentials, nil
}

// Save writes credentials to disk with user-only permissions.
func (s *fileStore) Save(credentials Credentials) error {
	if err := os.MkdirAll(filepath.Dir(s.path), credentialsDirMode); err != nil {
		return fmt.Errorf("failed to create openai credential directory: %w", err)
	}

	bytes, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode openai credentials: %w", err)
	}

	if err := os.WriteFile(s.path, bytes, credentialsFileMode); err != nil {
		return fmt.Errorf("failed to write openai credentials: %w", err)
	}

	return os.Chmod(s.path, credentialsFileMode)
}

// Remove deletes the credential file when it exists.
func (s *fileStore) Remove() error {
	if err := os.Remove(s.path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to remove openai credentials: %w", err)
	}

	return nil
}
