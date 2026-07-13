package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	grantTypeRefreshToken = "refresh_token"
	grantTypeDeviceCode   = "urn:ietf:params:oauth:grant-type:device_code"
	defaultExpiresIn      = 3600
)

type tokenClient struct {
	client   httpClient
	clientId string
	tokenUrl string
	now      func() time.Time
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type tokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Refresh exchanges expired Rovo Dev credentials for updated credentials.
func (c *tokenClient) Refresh(ctx context.Context, credentials internaloauth.Credentials) (internaloauth.Credentials, error) {
	form := url.Values{}
	form.Set("grant_type", grantTypeRefreshToken)
	form.Set("client_id", c.clientId)
	form.Set("refresh_token", credentials.Refresh)

	tokens, _, err := c.postTokenForm(ctx, form)
	if err != nil {
		return internaloauth.Credentials{}, err
	}

	return c.credentialsFromTokenResponse(tokens, credentials.Refresh), nil
}

// exchangeDeviceCode exchanges an approved device code for stored credentials.
func (c *tokenClient) exchangeDeviceCode(ctx context.Context, deviceCode string) (internaloauth.Credentials, internaloauth.DevicePollStatus, error) {
	form := url.Values{}
	form.Set("grant_type", grantTypeDeviceCode)
	form.Set("client_id", c.clientId)
	form.Set("device_code", deviceCode)

	tokens, tokenError, err := c.postTokenForm(ctx, form)
	if err != nil {
		return internaloauth.Credentials{}, "", err
	}

	if status, err := devicePollStatusFromTokenError(tokenError); status != internaloauth.DevicePollComplete || err != nil {
		return internaloauth.Credentials{}, status, err
	}

	return c.credentialsFromTokenResponse(tokens, ""), internaloauth.DevicePollComplete, nil
}

// devicePollStatusFromTokenError maps an OAuth device token error to a poll status.
func devicePollStatusFromTokenError(tokenError tokenErrorResponse) (internaloauth.DevicePollStatus, error) {
	switch tokenError.Error {
	case "":
		return internaloauth.DevicePollComplete, nil
	case "authorization_pending":
		return internaloauth.DevicePollPending, nil
	case "slow_down":
		return internaloauth.DevicePollSlowDown, nil
	case "access_denied":
		return internaloauth.DevicePollFailed, fmt.Errorf("rovo device authorization denied: %s", tokenErrorDetail(tokenError))
	case "expired_token":
		return internaloauth.DevicePollFailed, fmt.Errorf("rovo device code expired before approval: %s", tokenErrorDetail(tokenError))
	default:
		return internaloauth.DevicePollFailed, fmt.Errorf("rovo device authorization failed with %q: %s", tokenError.Error, tokenErrorDetail(tokenError))
	}
}

// tokenErrorDetail returns the token error description or a placeholder.
func tokenErrorDetail(tokenError tokenErrorResponse) string {
	if tokenError.ErrorDescription != "" {
		return tokenError.ErrorDescription
	}

	return "<no description>"
}

// postTokenForm posts a form-encoded body to the token endpoint and decodes the response.
func (c *tokenClient) postTokenForm(ctx context.Context, form url.Values) (tokenResponse, tokenErrorResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenUrl, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, tokenErrorResponse{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return tokenResponse{}, tokenErrorResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenResponse{}, tokenErrorResponse{}, err
	}

	if resp.StatusCode == http.StatusOK {
		var tokens tokenResponse

		if err := json.Unmarshal(responseBody, &tokens); err != nil {
			return tokenResponse{}, tokenErrorResponse{}, err
		}

		return tokens, tokenErrorResponse{}, nil
	}

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden {
		var tokenError tokenErrorResponse

		if err := json.Unmarshal(responseBody, &tokenError); err != nil {
			return tokenResponse{}, tokenErrorResponse{}, err
		}

		return tokenResponse{}, tokenError, nil
	}

	return tokenResponse{}, tokenErrorResponse{}, fmt.Errorf("rovo token endpoint returned HTTP %d: %s", resp.StatusCode, string(responseBody))
}

// credentialsFromTokenResponse converts an OAuth token response into stored credentials.
func (c *tokenClient) credentialsFromTokenResponse(tokens tokenResponse, fallbackRefresh string) internaloauth.Credentials {
	refresh := tokens.RefreshToken

	if refresh == "" {
		refresh = fallbackRefresh
	}

	expiresIn := tokens.ExpiresIn

	if expiresIn == 0 {
		expiresIn = defaultExpiresIn
	}

	return internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Refresh: refresh,
		Access:  tokens.AccessToken,
		Expires: c.now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(),
	}
}
