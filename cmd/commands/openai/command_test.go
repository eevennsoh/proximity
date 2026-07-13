package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/openai/internal/auth"
	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/proxy"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.uber.org/mock/gomock"
)

func TestCommandIncludesServeAndAuthActions(t *testing.T) {
	cmd := Command()

	assert.Equal(t, "openai", cmd.Name)
	require.Len(t, cmd.Subcommands, 5)

	names := make([]string, 0, len(cmd.Subcommands))

	for _, subcommand := range cmd.Subcommands {
		names = append(names, subcommand.Name)
	}

	assert.Contains(t, names, "serve")
	assert.Contains(t, names, "login")
	assert.Contains(t, names, "status")
	assert.Contains(t, names, "logout")
	assert.Contains(t, names, "doc")
}

func TestPortFlagDefaultsToAiGatewayPort(t *testing.T) {
	cmd := Command()

	flag, ok := cmd.Flags[0].(*cli.IntFlag)
	require.True(t, ok)
	assert.Equal(t, "port", flag.Name)
	assert.Equal(t, 29574, flag.Value)
}

// TestServePortCanBeSetBeforeOrAfterServe verifies both CLI port flag positions.
func TestServePortCanBeSetBeforeOrAfterServe(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "before serve",
			args: []string{"proximity", "openai", "--port", "29573", "serve"},
		},
		{
			name: "after serve",
			args: []string{"proximity", "openai", "serve", "--port", "29573"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var got int

			cmd := Command()
			for _, subcommand := range cmd.Subcommands {
				if subcommand.Name == "serve" {
					subcommand.Action = func(c *cli.Context) error {
						got = commandPort(c)
						return nil
					}
				}
			}

			app := cli.NewApp()
			app.Writer = io.Discard
			app.ErrWriter = io.Discard
			app.Commands = []*cli.Command{cmd}

			err := app.Run(testCase.args)

			require.NoError(t, err)
			assert.Equal(t, 29573, got)
		})
	}
}

// TestUnexpectedArgumentIsRejected verifies a mistyped subcommand fails loudly
// instead of silently running the parent action and dropping later flags.
func TestUnexpectedArgumentIsRejected(t *testing.T) {
	app := cli.NewApp()
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	app.Commands = []*cli.Command{Command()}

	err := app.Run([]string{"proximity", "openai", "server", "--port", "8092"})

	require.Error(t, err)
	assert.ErrorContains(t, err, `unexpected argument "server"`)
}

func TestDocCommandPrintsGeneratedEndpointReference(t *testing.T) {
	var output bytes.Buffer

	app := cli.NewApp()
	app.Writer = &output
	app.Commands = []*cli.Command{
		Command(),
	}

	err := app.Run([]string{"proximity", "openai", "--port", "3000", "doc"})
	require.NoError(t, err)

	docOutput := output.String()
	assert.NotContains(t, docOutput, "#")
	assert.Contains(t, docOutput, "Proximity OpenAI Proxy")
	assert.Contains(t, docOutput, "Base URL: http://localhost:3000")
	assert.Contains(t, docOutput, "METHOD")
	assert.Contains(t, docOutput, "/health")
	assert.Contains(t, docOutput, "Health check endpoint")
	assert.Contains(t, docOutput, "/v1/models")
	assert.Contains(t, docOutput, "List ChatGPT subscription models")
	assert.Contains(t, docOutput, "/v1/responses")
	assert.Contains(t, docOutput, "Responses endpoint")
	assert.Contains(t, docOutput, "/backend-api/codex/responses")
	assert.Contains(t, docOutput, "GET /health")
	assert.Contains(t, docOutput, "GET /v1/models")
	assert.Contains(t, docOutput, "POST /v1/responses")
	assert.Contains(t, docOutput, "curl http://localhost:3000/v1/responses")
	assert.NotContains(t, docOutput, "/v1/chat/completions")
}

func TestConfigTransformsResponsesRequestForCodexBackend(t *testing.T) {
	cfg, err := config.LoadFromBytes(proxyConfig)
	require.NoError(t, err)

	responsesOverride := cfg.Overrides.Uris["/v1/responses"]["POST"]

	assert.NotContains(t, responsesOverride.Request.Body.Expr, "openai")
	assert.Contains(t, responsesOverride.Request.Body.Expr, "instructions")
	assert.Contains(t, responsesOverride.Request.Body.Expr, "store")
	assert.Contains(t, responsesOverride.Request.Body.Expr, "stream")
}

func TestOpenaiTemplateOptionsRegisterExprHelpers(t *testing.T) {
	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Return("access-token", nil).Times(1)
	authService.EXPECT().AccountId(gomock.Any()).Return("account-123", nil).Times(1)

	renderer := proximitytemplate.NewRenderer(log.New(io.Discard, "", 0), openaiTemplateOptions(authService)...)

	accessToken, err := renderer.Render(context.Background(), "", `openaiAccessToken()`, make(map[string]any), nil)
	require.NoError(t, err)
	accountId, err := renderer.Render(context.Background(), "", `openaiAccountId()`, make(map[string]any), nil)

	require.NoError(t, err)
	assert.Equal(t, "access-token", string(accessToken))
	assert.Equal(t, "account-123", string(accountId))
}

func TestOpenaiTemplateOptionsRegisterTextTemplateHelpers(t *testing.T) {
	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Return("access-token", nil).Times(1)
	authService.EXPECT().AccountId(gomock.Any()).Return("account-123", nil).Times(1)

	renderer := proximitytemplate.NewRenderer(log.New(io.Discard, "", 0), openaiTemplateOptions(authService)...)

	accessToken, err := renderer.Render(context.Background(), `{{ openaiAccessToken }}`, "", make(map[string]any), nil)
	require.NoError(t, err)
	accountId, err := renderer.Render(context.Background(), `{{ openaiAccountId }}`, "", make(map[string]any), nil)

	require.NoError(t, err)
	assert.Equal(t, "access-token", string(accessToken))
	assert.Equal(t, "account-123", string(accountId))
}

func TestOpenaiResponsesNormalizesStringInputEndToEnd(t *testing.T) {
	// integration test: exercises the embedded OpenAI config through the real proxy and a local upstream.
	var upstreamPath string
	var upstreamHeaders http.Header
	var upstreamBody map[string]any

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamHeaders = r.Header.Clone()

		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bodyBytes, &upstreamBody))

		w.Header().Set("Content-Type", "")
		_, err = fmt.Fprint(w, "event: response.output_text.delta\n")
		require.NoError(t, err)
		_, err = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n")
		require.NoError(t, err)
		_, err = fmt.Fprint(w, "\n")
		require.NoError(t, err)
	}))
	defer upstream.Close()

	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Return("access-token", nil).Times(1)
	authService.EXPECT().AccountId(gomock.Any()).Return("account-123", nil).Times(1)

	baseUrl := startOpenaiProxy(t, upstream.URL, authService)

	requestBody := strings.NewReader(`{"model":"gpt-5.5","input":"hello","instructions":"","store":true,"stream":false}`)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseUrl+"/v1/responses", requestBody)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	responseBytes, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	assert.Equal(t, "/backend-api/codex/responses", upstreamPath)
	assert.Equal(t, "Bearer access-token", upstreamHeaders.Get("Authorization"))
	assert.Equal(t, "account-123", upstreamHeaders.Get("ChatGPT-Account-Id"))
	assert.Equal(t, "You are a helpful assistant.", upstreamBody["instructions"])
	assert.Equal(t, false, upstreamBody["store"])
	assert.Equal(t, true, upstreamBody["stream"])
	inputItems, ok := upstreamBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, inputItems, 1)

	userItem, ok := inputItems[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user", userItem["role"])
	assert.Equal(t, "hello", userItem["content"])
	assert.Equal(t, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", string(responseBytes))
}

func TestOpenaiResponsesStripsPersistedItemReferencesEndToEnd(t *testing.T) {
	// integration test: store:false requests must not send persisted item references to Codex.
	var upstreamBody map[string]any

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bodyBytes, &upstreamBody))

		w.Header().Set("Content-Type", "text/event-stream")
		_, err = fmt.Fprint(w, "data: {\"type\":\"response.output_text.done\"}\n")
		require.NoError(t, err)
	}))
	defer upstream.Close()

	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Return("access-token", nil).Times(1)
	authService.EXPECT().AccountId(gomock.Any()).Return("account-123", nil).Times(1)

	baseUrl := startOpenaiProxy(t, upstream.URL, authService)
	requestBody := strings.NewReader(`{"model":"gpt-5.5","input":[{"type":"reasoning","id":"rs_0890b8eab716ebb9016a2402df8fd081919001252ab66dab04","summary":[],"encrypted_content":"encrypted-state"},{"type":"item_reference","id":"rs_0890b8eab716ebb9016a2402df8fd081919001252ab66dab04"},{"role":"user","content":"continue"}],"store":true,"stream":false}`)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseUrl+"/v1/responses", requestBody)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	inputItems, ok := upstreamBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, inputItems, 2)

	reasoningItem, ok := inputItems[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "reasoning", reasoningItem["type"])
	assert.NotContains(t, reasoningItem, "id")
	assert.Equal(t, "encrypted-state", reasoningItem["encrypted_content"])

	userItem, ok := inputItems[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user", userItem["role"])
	assert.Equal(t, false, upstreamBody["store"])
	assert.Equal(t, true, upstreamBody["stream"])
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func TestOpenaiResponsesStreamsBeforeUpstreamCompletesEndToEnd(t *testing.T) {
	// integration test: config must request an upstream SSE response so the stable proxy streams it.
	var upstreamAccept string
	var upstreamAcceptEncoding string
	var upstreamSessionId string

	firstChunkFlushed := make(chan struct{})
	finishUpstream := make(chan struct{})
	finish := func() {
		select {
		case <-finishUpstream:
		default:
			close(finishUpstream)
		}
	}
	t.Cleanup(finish)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAccept = r.Header.Get("Accept")
		upstreamAcceptEncoding = r.Header.Get("Accept-Encoding")
		upstreamSessionId = r.Header.Get("session-id")

		_, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		if upstreamAccept == "text/event-stream" && upstreamAcceptEncoding == "identity" && upstreamSessionId == "session-123" {
			w.Header().Set("Content-Type", "text/event-stream")
		}

		_, err = fmt.Fprint(w, "event: response.output_text.delta\n")
		require.NoError(t, err)
		flusher.Flush()
		close(firstChunkFlushed)

		<-finishUpstream

		_, err = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
		require.NoError(t, err)
		flusher.Flush()
	}))
	defer upstream.Close()

	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Return("access-token", nil).Times(1)
	authService.EXPECT().AccountId(gomock.Any()).Return("account-123", nil).Times(1)

	baseUrl := startOpenaiProxy(t, upstream.URL, authService)
	requestBody := strings.NewReader(`{"model":"gpt-5.5","input":"hello","store":false,"stream":false}`)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseUrl+"/v1/responses", requestBody)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("session-id", "session-123")

	responseChan := make(chan *http.Response, 1)
	errorChan := make(chan error, 1)

	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			errorChan <- err
			return
		}

		responseChan <- response
	}()

	select {
	case <-firstChunkFlushed:
	case <-time.After(time.Second):
		t.Fatal("upstream did not flush the first chunk")
	}

	var response *http.Response

	select {
	case err := <-errorChan:
		require.NoError(t, err)
	case response = <-responseChan:
	case <-time.After(200 * time.Millisecond):
		finish()
		t.Fatal("proxy did not return response headers before upstream completed")
	}

	defer response.Body.Close()
	assert.Equal(t, "text/event-stream", upstreamAccept)
	assert.Equal(t, "identity", upstreamAcceptEncoding)
	assert.Equal(t, "session-123", upstreamSessionId)
	assert.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))

	line, err := bufio.NewReader(response.Body).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "event: response.output_text.delta\n", line)
	finish()
}

func TestOpenaiChatCompletionsIsNotRoutedEndToEnd(t *testing.T) {
	// integration test: verifies unsupported chat completions requests do not reach the Codex upstream.
	upstreamRequestCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Times(0)
	authService.EXPECT().AccountId(gomock.Any()).Times(0)

	baseUrl := startOpenaiProxy(t, upstream.URL, authService)

	requestBody := strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hello"}]}`)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseUrl+"/v1/chat/completions", requestBody)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
	assert.Equal(t, 0, upstreamRequestCount)
}

func TestConfigResponseContentTypeFallsBackForMissingUpstreamHeader(t *testing.T) {
	cfg, err := config.LoadFromBytes(proxyConfig)
	require.NoError(t, err)

	renderer := proximitytemplate.NewRenderer(log.Default())
	paths := []string{
		"/v1/responses",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			responseOverride := cfg.Overrides.Uris[path]["POST"]

			got, err := renderer.Render(
				context.Background(),
				"",
				responseOverride.Response.Headers[0].Expr,
				map[string]any{
					"headers": http.Header{},
				},
				nil,
			)

			require.NoError(t, err)
			assert.Equal(t, "text/event-stream", string(got))
		})
	}
}

func TestConfigResponsesDoesNotMutateResponseBody(t *testing.T) {
	cfg, err := config.LoadFromBytes(proxyConfig)
	require.NoError(t, err)

	responseOverride := cfg.Overrides.Uris["/v1/responses"]["POST"]

	assert.Empty(t, responseOverride.Response.Body.Expr)
	assert.Empty(t, responseOverride.Response.Body.Template)
	assert.Empty(t, responseOverride.Response.Body.Text)
	assert.Empty(t, responseOverride.Response.Body.Patches)
}

// startOpenaiProxy runs the embedded OpenAI config against a local upstream.
func startOpenaiProxy(t *testing.T, upstreamUrl string, authService auth.Interface) string {
	t.Helper()

	cfg, err := config.LoadFromBytes(proxyConfig)
	require.NoError(t, err)
	cfg.BaseEndpoint = strconv.Quote(upstreamUrl)

	port := unusedTcpPort(t)
	logger := log.New(io.Discard, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	proxyServer := proxy.New(proxy.Options{
		Port:            port,
		Logger:          logger,
		Config:          cfg,
		Vars:            make(map[string]any),
		TemplateOptions: openaiTemplateOptions(authService),
	})

	go proxyServer.RunServer(ctx)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		assert.NoError(t, proxyServer.Shutdown(shutdownCtx))
	})

	baseUrl := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForOpenaiProxy(t, baseUrl)
	return baseUrl
}

// unusedTcpPort reserves and releases a TCP port for a short-lived test server.
func unusedTcpPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	parsedPort, err := strconv.Atoi(port)
	require.NoError(t, err)
	return parsedPort
}

// waitForOpenaiProxy polls the health endpoint until the proxy accepts requests.
func waitForOpenaiProxy(t *testing.T, baseUrl string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		response, err := http.Get(baseUrl + "/health")
		if err == nil {
			_ = response.Body.Close()

			if response.StatusCode == http.StatusOK {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("proxy at %s did not become ready", baseUrl)
}
