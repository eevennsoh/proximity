package auth

import (
	"fmt"
	"net/http"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	providerName        = "anthropic"
	defaultClientId     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	defaultAuthorizeUrl = "https://claude.ai/oauth/authorize"
	defaultTokenUrl     = "https://platform.claude.com/v1/oauth/token"
	defaultOauthPort    = 53692
)

type options struct {
	credentialPath string
	client         httpClient
	clientId       string
	authorizeUrl   string
	tokenUrl       string
	oauthPort      int
	now            func() time.Time
	openBrowser    func(string) error
}

// Option configures the Anthropic auth service.
type Option func(*options)

// WithHttpClient overrides the HTTP client used for OAuth requests.
func WithHttpClient(client httpClient) Option {
	return func(options *options) {
		options.client = client
	}
}

// WithAuthorizeUrl overrides the OAuth authorization endpoint.
func WithAuthorizeUrl(authorizeUrl string) Option {
	return func(options *options) {
		options.authorizeUrl = authorizeUrl
	}
}

// WithTokenUrl overrides the OAuth token endpoint.
func WithTokenUrl(tokenUrl string) Option {
	return func(options *options) {
		options.tokenUrl = tokenUrl
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
		authorizeUrl:   defaultAuthorizeUrl,
		tokenUrl:       defaultTokenUrl,
		oauthPort:      defaultOauthPort,
		now:            time.Now,
		openBrowser: func(string) error {
			return fmt.Errorf("browser opener not configured")
		},
	}, nil
}
