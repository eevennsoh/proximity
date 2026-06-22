package oauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

const (
	defaultCallbackPath    = "/auth/callback"
	defaultCancelPath      = "/cancel"
	defaultSuccessMessage  = "OAuth login complete. You can close this tab."
	defaultErrorMessage    = "OAuth login failed."
	defaultCancelMessage   = "OAuth login cancelled."
	defaultMissingCodeText = "Missing OAuth code."
	defaultInvalidState    = "Invalid OAuth state."
)

// CallbackServerOptions configures a local OAuth callback server.
type CallbackServerOptions struct {
	Address        string
	CallbackPath   string
	CancelPath     string
	ExpectedState  string
	SuccessMessage string
	ErrorMessage   string
	CancelMessage  string
}

// CallbackResult contains the authorization code returned by the OAuth callback.
type CallbackResult struct {
	Code string
}

// CallbackServer waits for a single OAuth callback result.
type CallbackServer struct {
	server   *http.Server
	listener net.Listener
	result   chan callbackResult
}

type callbackResult struct {
	result CallbackResult
	err    error
}

// StartCallbackServer starts a local server that captures an OAuth authorization code.
func StartCallbackServer(options CallbackServerOptions) (*CallbackServer, error) {
	options = normalizeCallbackServerOptions(options)
	result := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(options.CallbackPath, callbackHandler(options, result))

	if options.CancelPath != "" {
		mux.HandleFunc(options.CancelPath, cancelHandler(options, result))
	}

	server := &http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp", options.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to start oauth callback server: %w", err)
	}

	callbackServer := &CallbackServer{
		server:   server,
		listener: listener,
		result:   result,
	}

	go serveCallbackServer(server, listener, result)
	return callbackServer, nil
}

// Address returns the bound listener address.
func (s *CallbackServer) Address() string {
	return s.listener.Addr().String()
}

// Wait blocks until the callback returns a result or the context ends.
func (s *CallbackServer) Wait(ctx context.Context) (CallbackResult, error) {
	select {
	case result := <-s.result:
		return result.result, result.err
	case <-ctx.Done():
		return CallbackResult{}, ctx.Err()
	}
}

// Close stops the local callback server.
func (s *CallbackServer) Close() error {
	return s.server.Close()
}

// normalizeCallbackServerOptions fills in default callback server options.
func normalizeCallbackServerOptions(options CallbackServerOptions) CallbackServerOptions {
	if options.Address == "" {
		options.Address = "localhost:0"
	}

	if options.CallbackPath == "" {
		options.CallbackPath = defaultCallbackPath
	}

	if options.CancelPath == "" {
		options.CancelPath = defaultCancelPath
	}

	if options.SuccessMessage == "" {
		options.SuccessMessage = defaultSuccessMessage
	}

	if options.ErrorMessage == "" {
		options.ErrorMessage = defaultErrorMessage
	}

	if options.CancelMessage == "" {
		options.CancelMessage = defaultCancelMessage
	}

	return options
}

// callbackHandler returns an HTTP handler for OAuth callback requests.
func callbackHandler(options CallbackServerOptions, result chan<- callbackResult) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		values := request.URL.Query()

		if values.Get("state") != options.ExpectedState {
			if err := writeCallbackError(response, http.StatusBadRequest, defaultInvalidState); err != nil {
				sendCallbackResult(result, CallbackResult{}, err)
				return
			}

			sendCallbackResult(result, CallbackResult{}, fmt.Errorf("invalid oauth state"))
			return
		}

		if oauthError := values.Get("error"); oauthError != "" {
			if err := writeCallbackError(response, http.StatusBadRequest, options.ErrorMessage); err != nil {
				sendCallbackResult(result, CallbackResult{}, err)
				return
			}

			sendCallbackResult(result, CallbackResult{}, fmt.Errorf("oauth callback error: %s", oauthError))
			return
		}

		code := values.Get("code")
		if code == "" {
			if err := writeCallbackError(response, http.StatusBadRequest, defaultMissingCodeText); err != nil {
				sendCallbackResult(result, CallbackResult{}, err)
				return
			}

			sendCallbackResult(result, CallbackResult{}, fmt.Errorf("missing oauth code"))
			return
		}

		response.WriteHeader(http.StatusOK)
		if _, err := response.Write([]byte(options.SuccessMessage)); err != nil {
			sendCallbackResult(result, CallbackResult{}, err)
			return
		}

		sendCallbackResult(result, CallbackResult{
			Code: code,
		}, nil)
	}
}

// cancelHandler returns an HTTP handler for user-cancelled OAuth flows.
func cancelHandler(options CallbackServerOptions, result chan<- callbackResult) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		if _, err := response.Write([]byte(options.CancelMessage)); err != nil {
			sendCallbackResult(result, CallbackResult{}, err)
			return
		}

		sendCallbackResult(result, CallbackResult{}, fmt.Errorf("oauth login cancelled"))
	}
}

// serveCallbackServer runs the local OAuth callback server and reports unexpected errors.
func serveCallbackServer(server *http.Server, listener net.Listener, result chan<- callbackResult) {
	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return
	}

	sendCallbackResult(result, CallbackResult{}, err)
}

// writeCallbackError writes a plain-text callback error response.
func writeCallbackError(response http.ResponseWriter, status int, message string) error {
	response.WriteHeader(status)
	_, err := response.Write([]byte(message))
	return err
}

// sendCallbackResult reports the first terminal callback result.
func sendCallbackResult(result chan<- callbackResult, callback CallbackResult, err error) {
	select {
	case result <- callbackResult{
		result: callback,
		err:    err,
	}:
	default:
	}
}
