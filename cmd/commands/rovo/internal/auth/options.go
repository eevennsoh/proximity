package auth

import (
	"fmt"
	"net/http"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	providerName = "rovo"

	environmentProd    = "prod"
	environmentStaging = "staging"

	defaultProdClientId    = "IN203ThI5wMOMPQTkbaLhq88Cdw4Lt4t"
	defaultStagingClientId = "Fnv3mouuy2F7n6hrJswwy6jhiA3mQblx"

	defaultProdDeviceCodeUrl    = "https://auth.atlassian.com/oauth/device/code"
	defaultProdTokenUrl         = "https://auth.atlassian.com/oauth/token"
	defaultStagingDeviceCodeUrl = "https://auth.stg.atlassian.com/oauth/device/code"
	defaultStagingTokenUrl      = "https://auth.stg.atlassian.com/oauth/token"
)

type environmentEndpoints struct {
	clientId      string
	deviceCodeUrl string
	tokenUrl      string
}

type options struct {
	credentialPath string
	client         httpClient
	clientId       string
	deviceCodeUrl  string
	tokenUrl       string
	now            func() time.Time
	openBrowser    func(string) error
}

// Option configures the Rovo Dev auth service.
type Option func(*options)

// WithHttpClient overrides the HTTP client used for OAuth requests.
func WithHttpClient(client httpClient) Option {
	return func(options *options) {
		options.client = client
	}
}

// WithClientId overrides the OAuth client identifier.
func WithClientId(clientId string) Option {
	return func(options *options) {
		options.clientId = clientId
	}
}

// WithDeviceCodeUrl overrides the device authorization endpoint.
func WithDeviceCodeUrl(deviceCodeUrl string) Option {
	return func(options *options) {
		options.deviceCodeUrl = deviceCodeUrl
	}
}

// WithTokenUrl overrides the OAuth token endpoint.
func WithTokenUrl(tokenUrl string) Option {
	return func(options *options) {
		options.tokenUrl = tokenUrl
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

	endpoints := environmentDefaults(environmentProd)

	return options{
		credentialPath: path,
		client:         http.DefaultClient,
		clientId:       endpoints.clientId,
		deviceCodeUrl:  endpoints.deviceCodeUrl,
		tokenUrl:       endpoints.tokenUrl,
		now:            time.Now,
		openBrowser: func(string) error {
			return fmt.Errorf("browser opener not configured")
		},
	}, nil
}

// environmentDefaults returns the client and endpoints for an OAuth environment.
func environmentDefaults(environment string) environmentEndpoints {
	if environment == environmentStaging {
		return environmentEndpoints{
			clientId:      defaultStagingClientId,
			deviceCodeUrl: defaultStagingDeviceCodeUrl,
			tokenUrl:      defaultStagingTokenUrl,
		}
	}

	return environmentEndpoints{
		clientId:      defaultProdClientId,
		deviceCodeUrl: defaultProdDeviceCodeUrl,
		tokenUrl:      defaultProdTokenUrl,
	}
}

// WithEnvironment selects the prod or staging Atlassian OAuth endpoints and client.
func WithEnvironment(environment string) Option {
	endpoints := environmentDefaults(environment)

	return func(options *options) {
		options.clientId = endpoints.clientId
		options.deviceCodeUrl = endpoints.deviceCodeUrl
		options.tokenUrl = endpoints.tokenUrl
	}
}
