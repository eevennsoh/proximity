package auth

import "time"

const (
	credentialTypeOauth = "oauth"
	refreshSafetyMargin = 30 * time.Second
)

// Credentials stores the ChatGPT OAuth data required to refresh and call upstream APIs.
type Credentials struct {
	Type      string `json:"type"`
	Refresh   string `json:"refresh"`
	Access    string `json:"access"`
	Expires   int64  `json:"expires"`
	AccountId string `json:"accountId,omitempty"`
}

// Status describes credential availability without exposing token values.
type Status struct {
	Authenticated bool
	Expired       bool
	AccountId     string
	ExpiresAt     time.Time
}

// Expired reports whether the access token should be refreshed before use.
func (c Credentials) Expired(now time.Time) bool {
	if c.Access == "" || c.Expires == 0 {
		return true
	}

	expiresAt := time.UnixMilli(c.Expires)
	return !now.Add(refreshSafetyMargin).Before(expiresAt)
}

// ExpiresAt returns the access token expiration time.
func (c Credentials) ExpiresAt() time.Time {
	if c.Expires == 0 {
		return time.Time{}
	}

	return time.UnixMilli(c.Expires)
}
