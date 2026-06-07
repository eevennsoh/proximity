package oauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type service struct {
	store     Store
	refresher Refresher
	now       func() time.Time
	margin    time.Duration

	refreshMu sync.Mutex
}

// New creates a generic OAuth service backed by the provided store and refresher.
func New(store Store, refresher Refresher, optionList ...Option) Interface {
	options := defaultOptions()

	for _, option := range optionList {
		option(&options)
	}

	return &service{
		store:     store,
		refresher: refresher,
		now:       options.now,
		margin:    options.refreshSafetyMargin,
	}
}

// AccessToken returns a valid access token, refreshing credentials when needed.
func (s *service) AccessToken(ctx context.Context) (string, error) {
	credentials, err := s.Credentials(ctx)
	if err != nil {
		return "", err
	}

	return credentials.Access, nil
}

// Credentials returns valid credentials, refreshing stored credentials when needed.
func (s *service) Credentials(ctx context.Context) (Credentials, error) {
	credentials, err := s.store.Load()
	if err != nil {
		return Credentials{}, err
	}

	if !credentials.expired(s.now(), s.margin) {
		return credentials, nil
	}

	return s.refreshCredentials(ctx, credentials)
}

// refreshCredentials refreshes expired credentials and saves the updated token set.
func (s *service) refreshCredentials(ctx context.Context, credentials Credentials) (Credentials, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	latest, err := s.store.Load()
	if err == nil {
		if !latest.expired(s.now(), s.margin) {
			return latest, nil
		}

		credentials = latest
	} else if !errors.Is(err, ErrCredentialsNotFound) {
		return Credentials{}, err
	}

	updated, err := s.refresher.Refresh(ctx, credentials)
	if err != nil {
		return Credentials{}, err
	}

	if err := validateRefreshedCredentials(updated); err != nil {
		return Credentials{}, err
	}

	if err := s.store.Save(updated); err != nil {
		return Credentials{}, err
	}

	return updated, nil
}

// validateRefreshedCredentials verifies a refresh result can be used and persisted.
func validateRefreshedCredentials(credentials Credentials) error {
	if credentials.Access == "" {
		return fmt.Errorf("oauth refresh returned empty access token")
	}

	if credentials.Refresh == "" {
		return fmt.Errorf("oauth refresh returned empty refresh token")
	}

	if credentials.Expires == 0 {
		return fmt.Errorf("oauth refresh returned empty expiry")
	}

	return nil
}
