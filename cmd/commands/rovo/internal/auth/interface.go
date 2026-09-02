package auth

import (
	"context"
	"io"
)

//go:generate mockgen -source=interface.go -destination=interface.mock.gen.go -package=auth

// Interface provides Atlassian OAuth operations for the Rovo Dev command and proxy runtime.
type Interface interface {
	// AccessToken returns a valid access token, refreshing credentials when needed.
	AccessToken(ctx context.Context) (string, error)
	// LoginWithDevice completes the Atlassian device-code flow and stores credentials.
	LoginWithDevice(ctx context.Context, output io.Writer) error
	// Status reports credential state without exposing token values.
	Status(ctx context.Context) (Status, error)
	// Logout removes stored credentials.
	Logout() error
}
