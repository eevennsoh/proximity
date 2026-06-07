package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	internaloauth "bitbucket.org/atlassian-developers/proximity/internal/oauth"
)

const (
	deviceUserCodePath = "/api/accounts/deviceauth/usercode"
	deviceTokenPath    = "/api/accounts/deviceauth/token"
	deviceCallbackUri  = "https://auth.openai.com/deviceauth/callback"
	deviceVerifyUrl    = "https://auth.openai.com/codex/device"
	devicePollTimeout  = 15 * time.Minute
	devicePollMargin   = 3 * time.Second
	deviceUserAgent    = "proximity"
)

type deviceUserCodeResponse struct {
	DeviceAuthId string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	Interval     string `json:"interval"`
}

type deviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

// loginWithDevice completes device-code OAuth and stores credentials.
func (s *service) loginWithDevice(ctx context.Context, output io.Writer) error {
	userCode, err := s.requestDeviceUserCode(ctx)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "Open %s and enter code: %s\n", deviceVerifyUrl, userCode.UserCode); err != nil {
		return err
	}

	token, err := s.pollDeviceAuthorization(ctx, userCode)
	if err != nil {
		return err
	}

	credentials, err := s.tokens.exchangeAuthorizationCode(ctx, token.AuthorizationCode, deviceCallbackUri, token.CodeVerifier)
	if err != nil {
		return err
	}

	if err := s.store.Save(credentials); err != nil {
		return err
	}

	return nil
}

// requestDeviceUserCode starts a device-code OAuth login and returns the user code.
func (s *service) requestDeviceUserCode(ctx context.Context) (deviceUserCodeResponse, error) {
	body := make(map[string]string)
	body["client_id"] = s.clientId

	var userCode deviceUserCodeResponse

	if err := s.postDeviceJson(ctx, deviceUserCodePath, body, &userCode); err != nil {
		return deviceUserCodeResponse{}, err
	}

	return userCode, nil
}

// pollDeviceAuthorization polls until the device login returns an authorization code.
func (s *service) pollDeviceAuthorization(ctx context.Context, userCode deviceUserCodeResponse) (deviceTokenResponse, error) {
	delay, err := devicePollDelay(userCode.Interval)
	if err != nil {
		return deviceTokenResponse{}, err
	}

	var token deviceTokenResponse

	err = internaloauth.PollDevice(ctx, internaloauth.DevicePollConfig{
		Interval: delay,
		Timeout:  devicePollTimeout,
	}, func(ctx context.Context) (internaloauth.DevicePollStatus, error) {
		polledToken, status, err := s.pollDeviceAuthorizationOnce(ctx, userCode)
		if err != nil {
			return "", err
		}

		if status == internaloauth.DevicePollComplete {
			token = polledToken
		}

		return status, nil
	})
	if err != nil {
		return deviceTokenResponse{}, err
	}

	return token, nil
}

// pollDeviceAuthorizationOnce performs one device authorization poll.
func (s *service) pollDeviceAuthorizationOnce(ctx context.Context, userCode deviceUserCodeResponse) (deviceTokenResponse, internaloauth.DevicePollStatus, error) {
	body := make(map[string]string)
	body["device_auth_id"] = userCode.DeviceAuthId
	body["user_code"] = userCode.UserCode

	encoded, err := json.Marshal(body)
	if err != nil {
		return deviceTokenResponse{}, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.issuer+deviceTokenPath, bytes.NewReader(encoded))
	if err != nil {
		return deviceTokenResponse{}, "", err
	}

	setDeviceHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return deviceTokenResponse{}, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return deviceTokenResponse{}, internaloauth.DevicePollPending, nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return deviceTokenResponse{}, "", err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return deviceTokenResponse{}, "", fmt.Errorf("openai device token endpoint returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var token deviceTokenResponse

	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return deviceTokenResponse{}, "", err
	}

	return token, internaloauth.DevicePollComplete, nil
}

// postDeviceJson posts JSON to a device auth endpoint and decodes the response.
func (s *service) postDeviceJson(ctx context.Context, path string, body any, target any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.issuer+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}

	setDeviceHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("openai device endpoint returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return json.Unmarshal(bodyBytes, target)
}

// setDeviceHeaders applies OpenCode-compatible device auth headers.
func setDeviceHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", deviceUserAgent)
}

// devicePollDelay returns the delay between device authorization polls.
func devicePollDelay(interval string) (time.Duration, error) {
	seconds, err := strconv.Atoi(interval)
	if err != nil {
		return 0, fmt.Errorf("invalid openai device polling interval: %w", err)
	}

	return time.Duration(seconds)*time.Second + devicePollMargin, nil
}
