package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const accountIdMetadataKey = "accountId"

// Status describes credential availability without exposing token values.
type Status struct {
	Authenticated bool
	Expired       bool
	AccountId     string
	ExpiresAt     time.Time
}

type service struct {
	store        internaloauth.Store
	oauthService internaloauth.Interface
	tokens       *tokenClient
	client       httpClient
	clientId     string
	issuer       string
	oauthPort    int
	now          func() time.Time
	openBrowser  func(string) error
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

	store := internaloauth.NewFileStore(options.credentialPath)
	tokens := &tokenClient{
		client:   options.client,
		clientId: options.clientId,
		issuer:   options.issuer,
		now:      options.now,
	}

	return &service{
		store: store,
		oauthService: internaloauth.New(
			store,
			tokens,
			internaloauth.WithNow(options.now),
		),
		tokens:      tokens,
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
	token, err := s.oauthService.AccessToken(ctx)
	if errors.Is(err, internaloauth.ErrCredentialsNotFound) {
		return "", fmt.Errorf("openai credentials not found; run proximity openai login or proximity openai login --device")
	}

	if err != nil {
		return "", err
	}

	return token, nil
}

// AccountId returns the stored account ID when available.
func (s *service) AccountId(ctx context.Context) (string, error) {
	credentials, err := s.oauthService.Credentials(ctx)
	if errors.Is(err, internaloauth.ErrCredentialsNotFound) {
		return "", fmt.Errorf("openai credentials not found; run proximity openai login or proximity openai login --device")
	}

	if err != nil {
		return "", err
	}

	return credentials.Metadata[accountIdMetadataKey], nil
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
		AccountId:     credentials.Metadata[accountIdMetadataKey],
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
