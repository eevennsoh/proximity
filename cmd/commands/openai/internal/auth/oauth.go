package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	oauthAuthorizePath  = "/oauth/authorize"
	oauthTokenPath      = "/oauth/token"
	oauthScope          = "openid profile email offline_access"
	browserLoginTimeout = 5 * time.Minute
	pkceVerifierLength  = 43
)

type tokenClient struct {
	client   httpClient
	clientId string
	issuer   string
	now      func() time.Time
}

type tokenResponse struct {
	IdToken      string `json:"id_token"`
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
		ExpectedState:  state,
		SuccessMessage: "OpenAI login complete. You can close this tab.",
		CancelMessage:  "OpenAI login cancelled.",
	})
	if err != nil {
		return fmt.Errorf("failed to start openai browser login callback server: %w", err)
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
		return fmt.Errorf("openai browser login timed out: %w", err)
	}

	if err != nil {
		return err
	}

	credentials, err := s.tokens.exchangeAuthorizationCode(ctx, result.Code, s.browserRedirectUri(), verifier)
	if err != nil {
		return err
	}

	return s.store.Save(credentials)
}

// Refresh exchanges expired OpenAI credentials for updated credentials.
func (c *tokenClient) Refresh(ctx context.Context, credentials internaloauth.Credentials) (internaloauth.Credentials, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", credentials.Refresh)
	form.Set("client_id", c.clientId)

	tokens, err := c.postTokenForm(ctx, form)
	if err != nil {
		return internaloauth.Credentials{}, err
	}

	return c.credentialsFromTokenResponse(tokens, credentials.Refresh), nil
}

// exchangeAuthorizationCode exchanges an authorization code for stored credentials.
func (c *tokenClient) exchangeAuthorizationCode(ctx context.Context, code string, redirectUri string, verifier string) (internaloauth.Credentials, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectUri)
	form.Set("client_id", c.clientId)
	form.Set("code_verifier", verifier)

	tokens, err := c.postTokenForm(ctx, form)
	if err != nil {
		return internaloauth.Credentials{}, err
	}

	return c.credentialsFromTokenResponse(tokens, ""), nil
}

// postTokenForm posts an OAuth form to the token endpoint and decodes the response.
func (c *tokenClient) postTokenForm(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+oauthTokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokenResponse{}, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return tokenResponse{}, fmt.Errorf("openai token endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokens tokenResponse

	if err := json.Unmarshal(body, &tokens); err != nil {
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

	credentials := internaloauth.Credentials{
		Type:    internaloauth.CredentialTypeOauth,
		Refresh: refresh,
		Access:  tokens.AccessToken,
		Expires: c.now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(),
	}

	accountId := accountIdFromTokenResponse(tokens)
	if accountId != "" {
		credentials.Metadata = make(map[string]string)
		credentials.Metadata[accountIdMetadataKey] = accountId
	}

	return credentials
}

// accountIdFromTokenResponse returns the first account ID exposed by the OAuth tokens.
func accountIdFromTokenResponse(tokens tokenResponse) string {
	accountId, err := extractAccountId(tokens.IdToken)
	if err == nil && accountId != "" {
		return accountId
	}

	accountId, err = extractAccountId(tokens.AccessToken)
	if err == nil {
		return accountId
	}

	return ""
}

// authorizationUrl returns the browser OAuth URL and verifier state.
func (s *service) authorizationUrl() (string, string, string, error) {
	pair, err := internaloauth.GeneratePkce(pkceVerifierLength)
	if err != nil {
		return "", "", "", err
	}

	state, err := internaloauth.RandomBase64Url(pkceVerifierLength)
	if err != nil {
		return "", "", "", err
	}

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", s.clientId)
	values.Set("redirect_uri", s.browserRedirectUri())
	values.Set("scope", oauthScope)
	values.Set("code_challenge", pair.Challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("id_token_add_organizations", "true")
	values.Set("codex_cli_simplified_flow", "true")
	values.Set("state", state)
	values.Set("originator", "opencode")

	return s.issuer + oauthAuthorizePath + "?" + values.Encode(), pair.Verifier, state, nil
}

// browserRedirectUri returns the local OAuth callback URL.
func (s *service) browserRedirectUri() string {
	return fmt.Sprintf("http://localhost:%d/auth/callback", s.oauthPort)
}
