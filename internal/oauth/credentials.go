package oauth

import "time"

const (
	CredentialTypeOauth = "oauth"
	refreshSafetyMargin = 30 * time.Second
)

// Credentials stores generic OAuth token data.
type Credentials struct {
	Type     string            `json:"type"`
	Refresh  string            `json:"refresh"`
	Access   string            `json:"access"`
	Expires  int64             `json:"expires"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Expired reports whether the access token should be refreshed before use.
func (c Credentials) Expired(now time.Time) bool {
	return c.expired(now, refreshSafetyMargin)
}

// ExpiresAt returns the access token expiration time.
func (c Credentials) ExpiresAt() time.Time {
	if c.Expires == 0 {
		return time.Time{}
	}

	return time.UnixMilli(c.Expires)
}

// expired reports whether the access token is inside the refresh margin.
func (c Credentials) expired(now time.Time, safetyMargin time.Duration) bool {
	if c.Access == "" || c.Expires == 0 {
		return true
	}

	expiresAt := time.UnixMilli(c.Expires)
	return !now.Add(safetyMargin).Before(expiresAt)
}
