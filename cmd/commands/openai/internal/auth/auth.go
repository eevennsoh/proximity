package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

type service struct {
	store       credentialStore
	client      httpClient
	clientId    string
	issuer      string
	oauthPort   int
	now         func() time.Time
	openBrowser func(string) error

	refreshMu sync.Mutex
}

// New creates an OpenAI auth service backed by Proximity credential storage.
func New(optionList ...Option) (Interface, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, err
	}

	for _, option := range optionList {
		option(&options)
	}

	return &service{
		store:       newFileStore(options.credentialPath),
		client:      options.client,
		clientId:    options.clientId,
		issuer:      options.issuer,
		oauthPort:   options.oauthPort,
		now:         options.now,
		openBrowser: options.openBrowser,
	}, nil
}

// AccessToken returns a valid access token, refreshing credentials when needed.
func (s *service) AccessToken(ctx context.Context) (string, error) {
	credentials, err := s.store.Load()
	if errors.Is(err, ErrCredentialsNotFound) {
		return "", fmt.Errorf("openai credentials not found; run proximity openai login or proximity openai login --device")
	}

	if err != nil {
		return "", err
	}

	if !credentials.Expired(s.now()) {
		return credentials.Access, nil
	}

	updated, err := s.refreshCredentials(ctx, credentials)
	if err != nil {
		return "", err
	}

	return updated.Access, nil
}

// AccountId returns the stored account ID when available.
func (s *service) AccountId(ctx context.Context) (string, error) {
	credentials, err := s.store.Load()
	if errors.Is(err, ErrCredentialsNotFound) {
		return "", fmt.Errorf("openai credentials not found; run proximity openai login or proximity openai login --device")
	}

	if err != nil {
		return "", err
	}

	if credentials.AccountId != "" {
		return credentials.AccountId, nil
	}

	if credentials.Expired(s.now()) {
		credentials, err = s.refreshCredentials(ctx, credentials)
		if err != nil {
			return "", err
		}
	}

	return credentials.AccountId, nil
}

// Status reports credential state without exposing token values.
func (s *service) Status(context.Context) (Status, error) {
	credentials, err := s.store.Load()
	if errors.Is(err, ErrCredentialsNotFound) {
		return Status{}, nil
	}

	if err != nil {
		return Status{}, err
	}

	return Status{
		Authenticated: true,
		Expired:       credentials.Expired(s.now()),
		AccountId:     credentials.AccountId,
		ExpiresAt:     credentials.ExpiresAt(),
	}, nil
}

// Logout removes stored credentials.
func (s *service) Logout() error {
	return s.store.Remove()
}

// LoginWithBrowser completes browser OAuth and stores credentials.
func (s *service) LoginWithBrowser(ctx context.Context, output io.Writer) error {
	return s.loginWithBrowser(ctx, output)
}

// LoginWithDevice completes device-code OAuth and stores credentials.
func (s *service) LoginWithDevice(ctx context.Context, output io.Writer) error {
	return s.loginWithDevice(ctx, output)
}
