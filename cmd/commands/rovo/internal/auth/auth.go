package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

// Status describes credential availability without exposing token values.
type Status struct {
	Authenticated bool
	Expired       bool
	ExpiresAt     time.Time
}

type service struct {
	store         internaloauth.Store
	oauthService  internaloauth.Interface
	tokens        *tokenClient
	clientId      string
	deviceCodeUrl string
	now           func() time.Time
	openBrowser   func(string) error
}

// New creates a Rovo Dev auth service backed by Proximity credential storage.
func New(optionList ...Option) (Interface, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, err
	}

	for _, option := range optionList {
		option(&options)
	}

	store := internaloauth.NewFileStore(options.credentialPath)
	tokens := &tokenClient{
		client:   options.client,
		clientId: options.clientId,
		tokenUrl: options.tokenUrl,
		now:      options.now,
	}

	return &service{
		store: store,
		oauthService: internaloauth.New(
			store,
			tokens,
			internaloauth.WithNow(options.now),
		),
		tokens:        tokens,
		clientId:      options.clientId,
		deviceCodeUrl: options.deviceCodeUrl,
		now:           options.now,
		openBrowser:   options.openBrowser,
	}, nil
}

// AccessToken returns a valid access token, refreshing credentials when needed.
func (s *service) AccessToken(ctx context.Context) (string, error) {
	token, err := s.oauthService.AccessToken(ctx)
	if errors.Is(err, internaloauth.ErrCredentialsNotFound) {
		return "", fmt.Errorf("rovo credentials not found; run proximity rovo login")
	}

	if err != nil {
		return "", err
	}

	return token, nil
}

// Status reports credential state without exposing token values.
func (s *service) Status(context.Context) (Status, error) {
	credentials, err := s.store.Load()
	if errors.Is(err, internaloauth.ErrCredentialsNotFound) {
		return Status{}, nil
	}

	if err != nil {
		return Status{}, err
	}

	return Status{
		Authenticated: true,
		Expired:       credentials.Expired(s.now()),
		ExpiresAt:     credentials.ExpiresAt(),
	}, nil
}

// Logout removes stored credentials.
func (s *service) Logout() error {
	return s.store.Remove()
}

// LoginWithDevice completes the Atlassian device-code flow and stores credentials.
func (s *service) LoginWithDevice(ctx context.Context, output io.Writer) error {
	return s.loginWithDevice(ctx, output)
}
