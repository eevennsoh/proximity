package oauth

import "context"

//go:generate mockgen -source=interface.go -destination=interface.mock.gen.go -package=oauth

// Interface provides generic OAuth credential access.
type Interface interface {
	// AccessToken returns a valid access token, refreshing credentials when needed.
	AccessToken(ctx context.Context) (string, error)
	// Credentials returns valid credentials, refreshing stored credentials when needed.
	Credentials(ctx context.Context) (Credentials, error)
}

// Store persists OAuth credentials.
type Store interface {
	// Load reads credentials from storage.
	Load() (Credentials, error)
	// Save writes credentials to storage.
	Save(credentials Credentials) error
	// Remove deletes credentials from storage.
	Remove() error
}

// Refresher exchanges expired credentials for updated credentials.
type Refresher interface {
	// Refresh returns updated credentials for the same OAuth identity.
	Refresh(ctx context.Context, credentials Credentials) (Credentials, error)
}
