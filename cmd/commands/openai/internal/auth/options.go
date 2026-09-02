package auth

import (
	"fmt"
	"net/http"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	providerName     = "openai"
	defaultClientId  = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultIssuer    = "https://auth.openai.com"
	defaultOauthPort = 1455
)

type options struct {
	credentialPath string
	client         httpClient
	clientId       string
	issuer         string
	oauthPort      int
	now            func() time.Time
	openBrowser    func(string) error
}

// Option configures the OpenAI auth service.
type Option func(*options)

// WithHttpClient overrides the HTTP client used for OAuth requests.
func WithHttpClient(client httpClient) Option {
	return func(options *options) {
		options.client = client
	}
}

// WithIssuer overrides the OAuth issuer.
func WithIssuer(issuer string) Option {
	return func(options *options) {
		options.issuer = issuer
	}
}

// WithOauthPort overrides the local browser callback port.
func WithOauthPort(port int) Option {
	return func(options *options) {
		options.oauthPort = port
	}
}

// WithNow overrides the service clock.
func WithNow(now func() time.Time) Option {
	return func(options *options) {
		options.now = now
	}
}

// WithBrowserOpener overrides how browser URLs are opened.
func WithBrowserOpener(openBrowser func(string) error) Option {
	return func(options *options) {
		options.openBrowser = openBrowser
	}
}

// defaultOptions returns production defaults for OAuth and credential storage.
func defaultOptions() (options, error) {
	path, err := internaloauth.DefaultCredentialPath(providerName)
	if err != nil {
		return options{}, err
	}

	return options{
		credentialPath: path,
		client:         http.DefaultClient,
		clientId:       defaultClientId,
		issuer:         defaultIssuer,
		oauthPort:      defaultOauthPort,
		now:            time.Now,
		openBrowser: func(string) error {
			return fmt.Errorf("browser opener not configured")
		},
	}, nil
}
