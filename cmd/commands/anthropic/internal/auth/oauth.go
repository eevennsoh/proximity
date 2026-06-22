package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	oauthAuthorizePath  = "/oauth/authorize"
	oauthTokenPath      = "/v1/oauth/token"
	oauthCallbackPath   = "/callback"
	oauthScope          = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	anthropicUserAgent  = "claude-cli/1.0.0"
	anthropicAppHeader  = "cli"
	browserLoginTimeout = 5 * time.Minute
	pkceVerifierLength  = 43
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

// loginWithBrowser completes browser OAuth and stores credentials.
func (s *service) loginWithBrowser(ctx context.Context, output io.Writer) error {
	authorizeUrl, verifier, state, err := s.authorizationUrl()
	if err != nil {
		return err
	}

	callbackServer, err := internaloauth.StartCallbackServer(internaloauth.CallbackServerOptions{
		Address:        fmt.Sprintf("localhost:%d", s.oauthPort),
		CallbackPath:   oauthCallbackPath,
		ExpectedState:  state,
		SuccessMessage: "Anthropic login complete. You can close this tab.",
		CancelMessage:  "Anthropic login cancelled.",
	})
	if err != nil {
		return fmt.Errorf("failed to start anthropic browser login callback server: %w", err)
	}
	defer callbackServer.Close()

	if _, err := fmt.Fprintf(output, "opened browser: %s\n", authorizeUrl); err != nil {
		return err
	}

	if err := s.openBrowser(authorizeUrl); err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, browserLoginTimeout)
	defer cancel()

	result, err := callbackServer.Wait(waitCtx)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("anthropic browser login timed out: %w", err)
	}

	if err != nil {
		return err
	}

	credentials, err := s.tokens.exchangeAuthorizationCode(ctx, result.Code, verifier, s.browserRedirectUri())
	if err != nil {
		return err
	}

	return s.store.Save(credentials)
}

// Refresh exchanges expired Anthropic credentials for updated credentials.
func (c *tokenClient) Refresh(ctx context.Context, credentials internaloauth.Credentials) (internaloauth.Credentials, error) {
	body := make(map[string]string)
	body["grant_type"] = "refresh_token"
	body["client_id"] = c.clientId
	body["refresh_token"] = credentials.Refresh

	tokens, err := c.postTokenJson(ctx, body)
	if err != nil {
		return internaloauth.Credentials{}, err
	}

	return c.credentialsFromTokenResponse(tokens, credentials.Refresh), nil
}

// exchangeAuthorizationCode exchanges an authorization code for stored credentials.
func (c *tokenClient) exchangeAuthorizationCode(ctx context.Context, code string, verifier string, redirectUri string) (internaloauth.Credentials, error) {
	body := make(map[string]string)
	body["grant_type"] = "authorization_code"
	body["client_id"] = c.clientId
	body["code"] = code
	body["state"] = verifier
	body["redirect_uri"] = redirectUri
	body["code_verifier"] = verifier

	tokens, err := c.postTokenJson(ctx, body)
	if err != nil {
		return internaloauth.Credentials{}, err
	}

	return c.credentialsFromTokenResponse(tokens, ""), nil
}

// postTokenJson posts an OAuth JSON body to the token endpoint and decodes the response.
func (c *tokenClient) postTokenJson(ctx context.Context, body map[string]string) (tokenResponse, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return tokenResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenUrl, bytes.NewReader(encoded))
	if err != nil {
		return tokenResponse{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", anthropicUserAgent)
	req.Header.Set("X-App", anthropicAppHeader)

	resp, err := c.client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenResponse{}, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return tokenResponse{}, fmt.Errorf("anthropic token endpoint returned HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	var tokens tokenResponse

	if err := json.Unmarshal(responseBody, &tokens); err != nil {
		return tokenResponse{}, err
	}

	return tokens, nil
}

// credentialsFromTokenResponse converts an OAuth token response into stored credentials.
func (c *tokenClient) credentialsFromTokenResponse(tokens tokenResponse, fallbackRefresh string) internaloauth.Credentials {
	refresh := tokens.RefreshToken

	if refresh == "" {
		refresh = fallbackRefresh
	}

	expiresIn := tokens.ExpiresIn

	if expiresIn == 0 {
		expiresIn = 3600
	}

	return internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Refresh: refresh,
		Access:  tokens.AccessToken,
		Expires: c.now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(),
	}
}

// authorizationUrl returns the browser OAuth URL and verifier state.
func (s *service) authorizationUrl() (string, string, string, error) {
	pair, err := internaloauth.GeneratePkce(pkceVerifierLength)
	if err != nil {
		return "", "", "", err
	}

	state := pair.Verifier
	values := url.Values{}
	values.Set("code", "true")
	values.Set("response_type", "code")
	values.Set("client_id", s.clientId)
	values.Set("redirect_uri", s.browserRedirectUri())
	values.Set("scope", oauthScope)
	values.Set("code_challenge", pair.Challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("state", state)

	return s.authorizeUrl + "?" + values.Encode(), pair.Verifier, state, nil
}

// browserRedirectUri returns the local OAuth callback URL.
func (s *service) browserRedirectUri() string {
	return fmt.Sprintf("http://localhost:%d%s", s.oauthPort, oauthCallbackPath)
}
