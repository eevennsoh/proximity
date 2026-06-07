package oauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallbackServerReturnsAuthorizationCode verifies state validation and code delivery.
func TestCallbackServerReturnsAuthorizationCode(t *testing.T) {
	server, err := StartCallbackServer(CallbackServerOptions{
		Address:        "127.0.0.1:0",
		CallbackPath:   "/auth/callback",
		CancelPath:     "/cancel",
		ExpectedState:  "state-123",
		SuccessMessage: "login complete",
		CancelMessage:  "login cancelled",
	})
	require.NoError(t, err)
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + server.Address() + "/auth/callback?code=auth-code&state=state-123")
		if err != nil {
			done <- err
			return
		}
		defer response.Body.Close()

		body, err := io.ReadAll(response.Body)
		if err != nil {
			done <- err
			return
		}

		if response.StatusCode != http.StatusOK {
			done <- fmt.Errorf("status code = %d", response.StatusCode)
			return
		}

		if string(body) != "login complete" {
			done <- fmt.Errorf("body = %q", string(body))
			return
		}

		done <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := server.Wait(ctx)

	require.NoError(t, err)
	require.NoError(t, <-done)
	assert.Equal(t, "auth-code", result.Code)
}

// TestCallbackServerRejectsInvalidState verifies mismatched callback state fails.
func TestCallbackServerRejectsInvalidState(t *testing.T) {
	server, err := StartCallbackServer(CallbackServerOptions{
		Address:       "127.0.0.1:0",
		CallbackPath:  "/auth/callback",
		ExpectedState: "state-123",
	})
	require.NoError(t, err)
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + server.Address() + "/auth/callback?code=auth-code&state=wrong")
		if err != nil {
			done <- err
			return
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusBadRequest {
			done <- fmt.Errorf("status code = %d", response.StatusCode)
			return
		}

		done <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = server.Wait(ctx)

	require.NoError(t, <-done)
	assert.ErrorContains(t, err, "invalid oauth state")
}
