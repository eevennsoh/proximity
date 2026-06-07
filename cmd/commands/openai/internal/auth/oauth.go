package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	oauthAuthorizePath  = "/oauth/authorize"
	oauthTokenPath      = "/oauth/token"
	oauthScope          = "openid profile email offline_access"
	browserLoginTimeout = 5 * time.Minute
	pkceVerifierLength  = 43
)

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

	result := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(response http.ResponseWriter, request *http.Request) {
		err := s.handleBrowserCallback(request.Context(), response, request, verifier, state)
		sendBrowserResult(result, err)
	})
	mux.HandleFunc("/cancel", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		if _, err := response.Write([]byte("OpenAI login cancelled.")); err != nil {
			return
		}

		sendBrowserResult(result, fmt.Errorf("openai browser login cancelled"))
	})

	server := &http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", s.oauthPort))
	if err != nil {
		return fmt.Errorf("failed to start openai browser login callback server: %w", err)
	}
	defer server.Close()

	serverErr := make(chan error, 1)
	go serveBrowserCallback(server, listener, serverErr)

	fmt.Fprintf(output, "opened browser: %s\n", authorizeUrl)
	if err := s.openBrowser(authorizeUrl); err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, browserLoginTimeout)
	defer cancel()

	select {
	case err := <-result:
		return err
	case err := <-serverErr:
		if err != nil {
			return err
		}

		return fmt.Errorf("openai browser login callback server stopped")
	case <-waitCtx.Done():
		return fmt.Errorf("openai browser login timed out: %w", waitCtx.Err())
	}
}

// handleBrowserCallback validates callback state, exchanges the code, and saves credentials.
func (s *service) handleBrowserCallback(ctx context.Context, response http.ResponseWriter, request *http.Request, verifier string, state string) error {
	values := request.URL.Query()
	if values.Get("state") != state {
		response.WriteHeader(http.StatusBadRequest)
		if _, err := response.Write([]byte("Invalid OAuth state.")); err != nil {
			return err
		}

		return fmt.Errorf("invalid openai oauth state")
	}

	code := values.Get("code")
	if code == "" {
		response.WriteHeader(http.StatusBadRequest)
		if _, err := response.Write([]byte("Missing OAuth code.")); err != nil {
			return err
		}

		return fmt.Errorf("missing openai oauth code")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.browserRedirectUri())
	form.Set("client_id", s.clientId)
	form.Set("code_verifier", verifier)

	tokens, err := s.postTokenForm(ctx, form)
	if err != nil {
		response.WriteHeader(http.StatusBadGateway)
		if _, writeErr := response.Write([]byte("Failed to exchange OAuth code.")); writeErr != nil {
			return writeErr
		}

		return err
	}

	credentials := s.credentialsFromTokenResponse(tokens, "")
	if err := s.store.Save(credentials); err != nil {
		response.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := response.Write([]byte("Failed to store OAuth credentials.")); writeErr != nil {
			return writeErr
		}

		return err
	}

	response.WriteHeader(http.StatusOK)
	if _, err := response.Write([]byte("OpenAI login complete. You can close this tab.")); err != nil {
		return err
	}

	return nil
}

// serveBrowserCallback runs the local OAuth callback server and reports unexpected errors.
func serveBrowserCallback(server *http.Server, listener net.Listener, result chan<- error) {
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		result <- nil
		return
	}

	result <- err
}

// sendBrowserResult reports the first terminal browser login result.
func sendBrowserResult(result chan<- error, err error) {
	select {
	case result <- err:
	default:
	}
}

// refreshCredentials refreshes expired credentials and saves the updated token set.
func (s *service) refreshCredentials(ctx context.Context, credentials Credentials) (Credentials, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	latest, err := s.store.Load()
	if err == nil && !latest.Expired(s.now()) {
		return latest, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", credentials.Refresh)
	form.Set("client_id", s.clientId)

	tokens, err := s.postTokenForm(ctx, form)
	if err != nil {
		return Credentials{}, err
	}

	updated := s.credentialsFromTokenResponse(tokens, credentials.Refresh)
	if err := s.store.Save(updated); err != nil {
		return Credentials{}, err
	}

	return updated, nil
}

// postTokenForm posts an OAuth form to the token endpoint and decodes the response.
func (s *service) postTokenForm(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.issuer+oauthTokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
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
func (s *service) credentialsFromTokenResponse(tokens tokenResponse, fallbackRefresh string) Credentials {
	refresh := tokens.RefreshToken

	if refresh == "" {
		refresh = fallbackRefresh
	}

	expiresIn := tokens.ExpiresIn

	if expiresIn == 0 {
		expiresIn = 3600
	}

	return Credentials{
		Type:      credentialTypeOauth,
		Refresh:   refresh,
		Access:    tokens.AccessToken,
		Expires:   s.now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(),
		AccountId: accountIdFromTokenResponse(tokens),
	}
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
	verifier, err := randomBase64Url(pkceVerifierLength)
	if err != nil {
		return "", "", "", err
	}

	state, err := randomBase64Url(pkceVerifierLength)
	if err != nil {
		return "", "", "", err
	}

	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])

	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", s.clientId)
	values.Set("redirect_uri", s.browserRedirectUri())
	values.Set("scope", oauthScope)
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("id_token_add_organizations", "true")
	values.Set("codex_cli_simplified_flow", "true")
	values.Set("state", state)
	values.Set("originator", "opencode")

	return s.issuer + oauthAuthorizePath + "?" + values.Encode(), verifier, state, nil
}

// browserRedirectUri returns the local OAuth callback URL.
func (s *service) browserRedirectUri() string {
	return fmt.Sprintf("http://localhost:%d/auth/callback", s.oauthPort)
}

// randomBase64Url returns a URL-safe random string of the requested length.
func randomBase64Url(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	if len(encoded) > length {
		return encoded[:length], nil
	}

	return encoded, nil
}
