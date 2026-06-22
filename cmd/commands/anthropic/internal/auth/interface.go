package auth

import (
	"context"
	"io"
)

//go:generate mockgen -source=interface.go -destination=interface.mock.gen.go -package=auth

// Interface provides Claude OAuth operations for the Anthropic command and proxy runtime.
type Interface interface {
	// AccessToken returns a valid access token, refreshing credentials when needed.
	AccessToken(ctx context.Context) (string, error)
	// LoginWithBrowser completes browser OAuth and stores the resulting credentials.
	LoginWithBrowser(ctx context.Context, output io.Writer) error
	// Status reports credential state without exposing token values.
	Status(ctx context.Context) (Status, error)
	// Logout removes stored credentials.
	Logout() error
}
