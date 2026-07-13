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
	devicePollTimeout   = 15 * time.Minute
	defaultPollInterval = 5 * time.Second
)

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationUri         string `json:"verification_uri"`
	VerificationUriComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// verificationUrl returns the user-facing URL, preferring the pre-filled variant.
func (r deviceCodeResponse) verificationUrl() string {
	if r.VerificationUriComplete != "" {
		return r.VerificationUriComplete
	}

	return r.VerificationUri
}

// pollInterval returns the server-supplied polling interval or a default.
func (r deviceCodeResponse) pollInterval() time.Duration {
	if r.Interval <= 0 {
		return defaultPollInterval
	}

	return time.Duration(r.Interval) * time.Second
}

// loginWithDevice completes the Atlassian device-code flow and stores credentials.
func (s *service) loginWithDevice(ctx context.Context, output io.Writer) error {
	device, err := s.requestDeviceCode(ctx)
	if err != nil {
		return err
	}

	if err := s.presentVerification(output, device); err != nil {
		return err
	}

	credentials, err := s.pollDeviceAuthorization(ctx, device)
	if err != nil {
		return err
	}

	return s.store.Save(credentials)
}

// presentVerification shows the verification URL and code, opening a browser when possible.
func (s *service) presentVerification(output io.Writer, device deviceCodeResponse) error {
	if _, err := fmt.Fprintf(output, "Open %s and enter code: %s\n", device.verificationUrl(), device.UserCode); err != nil {
		return err
	}

	if err := s.openBrowser(device.verificationUrl()); err != nil {
		_, printErr := fmt.Fprintf(output, "could not open browser automatically: %v\n", err)
		return printErr
	}

	return nil
}

// requestDeviceCode starts the device authorization flow and returns the device code session.
func (s *service) requestDeviceCode(ctx context.Context) (deviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", s.clientId)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.deviceCodeUrl, strings.NewReader(form.Encode()))
	if err != nil {
		return deviceCodeResponse{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.tokens.client.Do(req)
	if err != nil {
		return deviceCodeResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return deviceCodeResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return deviceCodeResponse{}, fmt.Errorf("rovo device authorization endpoint returned HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	var device deviceCodeResponse

	if err := json.Unmarshal(responseBody, &device); err != nil {
		return deviceCodeResponse{}, err
	}

	return device, nil
}

// pollDeviceAuthorization polls the token endpoint until the device login is approved.
func (s *service) pollDeviceAuthorization(ctx context.Context, device deviceCodeResponse) (internaloauth.Credentials, error) {
	var credentials internaloauth.Credentials

	err := internaloauth.PollDevice(ctx, internaloauth.DevicePollConfig{
		Interval: device.pollInterval(),
		Timeout:  devicePollTimeout,
	}, func(ctx context.Context) (internaloauth.DevicePollStatus, error) {
		polled, status, err := s.tokens.exchangeDeviceCode(ctx, device.DeviceCode)
		if err != nil {
			return "", err
		}

		if status == internaloauth.DevicePollComplete {
			credentials = polled
		}

		return status, nil
	})
	if err != nil {
		return internaloauth.Credentials{}, err
	}

	return credentials, nil
}
