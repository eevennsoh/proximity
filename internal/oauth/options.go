package oauth

import "time"

type options struct {
	now                 func() time.Time
	refreshSafetyMargin time.Duration
}

// Option configures the OAuth service.
type Option func(*options)

// WithNow overrides the service clock.
func WithNow(now func() time.Time) Option {
	return func(options *options) {
		options.now = now
	}
}

// WithRefreshSafetyMargin overrides the early-refresh window before token expiry.
func WithRefreshSafetyMargin(safetyMargin time.Duration) Option {
	return func(options *options) {
		options.refreshSafetyMargin = safetyMargin
	}
}

// defaultOptions returns production defaults for the OAuth service.
func defaultOptions() options {
	return options{
		now:                 time.Now,
		refreshSafetyMargin: refreshSafetyMargin,
	}
}
