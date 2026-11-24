package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	nativeV2Path = "/v2/responses"
	openAIV2Path = "/openai-v2/v1/responses"
	claudeV2Path = "/claude-v2/v1/messages"
)

type gatewayRequest struct {
	path, raw string
	body      jsonObject
	headers   http.Header
}

func gatewayProxy(t *testing.T, cfgPath string, handler func(http.ResponseWriter, *http.Request, gatewayRequest)) string {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		body, err := decodeObject(strings.NewReader(string(data)))
		assert.NoError(t, err)
		handler(w, r, gatewayRequest{r.URL.Path, string(data), body, r.Header.Clone()})
	}))
	t.Cleanup(upstream.Close)
	cfg, err := config.Load(filepath.Join("..", "..", cfgPath))
	require.NoError(t, err)
	cfg.BaseEndpoint = strconv.Quote(upstream.URL)
	for i, header := range cfg.Overrides.Global.Request.Headers {
		if header.Name == "Authorization" {
			cfg.Overrides.Global.Request.Headers[i].Input = config.Input{Text: "slauth test"}
		}
	}
	s := New(Options{
		Logger: log.New(io.Discard, "", 0), Config: cfg,
		Vars: map[string]any{
			"atlassianCloudId": "fallback-cloud",
			"profiles": []any{
				map[string]any{"name": "default", "useCaseId": "default-use-case"},
				map[string]any{"name": "selected", "useCaseId": "selected-use-case", "atlassianCloudId": "selected-cloud"},
			},
		},
	}).(*server)
	routes, err := s.combineCommonUriConfigs(context.Background())
	require.NoError(t, err)
	for path, methods := range routes {
		for method, route := range methods {
			s.router.Method(method, path, s.handleEndpoint(route))
		}
	}
	local := httptest.NewServer(s.router)
	t.Cleanup(local.Close)
	return local.URL
}

func postGateway(t *testing.T, baseURL, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Atlassian-Session-Context", "opaque-session")
	req.Header.Set("Anthropic-Beta", "test-beta")
	req.Header.Set("X-Api-Key", "local-key")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestGatewayV2RoutesPreserveProfilesAndV1Defaults(t *testing.T) {
	const nativeRequest = `{"model":"anthropic/claude-sonnet-4-6", "input":"Say ready", "stream":true}`
	const nativeStream = ":heartbeat\r\n\r\ndata: {\"type\":\"response.completed\"}\r\n\r\n"
	const compatibleResponse = `{"id":"resp_1","status":"completed","output":[{"type":"platform:provider_block","provider":"openai","data":{"type":"web_search_call","id":"search_1"}},{"type":"message","content":[{"type":"output_text","text":"ready"}]}]}`
	for _, path := range []string{"config.yaml", "cmd/commands/ai-gateway/config.yaml"} {
		t.Run(path, func(t *testing.T) {
			requests := make(chan gatewayRequest, 12)
			base := gatewayProxy(t, path, func(w http.ResponseWriter, r *http.Request, request gatewayRequest) {
				requests <- request
				w.Header().Set("X-Atlassian-Session-Context", "next-session")
				w.Header().Set("X-Atlassian-Serving-Vendor", "google")
				w.Header().Set("Content-Type", "application/json")
				switch {
				case request.raw == nativeRequest:
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, nativeStream)
				case request.body["model"] == "force-upstream-error":
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"requestId":"gateway-123","code":"invalid_request","message":"Invalid model"}`)
				case request.body["stream"] == true:
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":"+compatibleResponse+"}\n\n")
				case r.URL.Path == nativeV2Path:
					fmt.Fprint(w, compatibleResponse)
				default:
					fmt.Fprint(w, `{"called":"v1"}`)
				}
			})
			res := postGateway(t, base, "/p/selected"+nativeV2Path, nativeRequest)
			require.Equal(t, 200, res.StatusCode)
			assert.Equal(t, "next-session", res.Header.Get("X-Atlassian-Session-Context"))
			assert.Equal(t, "google", res.Header.Get("X-Atlassian-Serving-Vendor"))
			native, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, nativeStream, string(native))
			seen := <-requests
			assert.Equal(t, nativeV2Path, seen.path)
			assert.Equal(t, nativeRequest, seen.raw)
			assert.Equal(t, "selected-cloud", seen.headers.Get("X-Atlassian-CloudId"))
			assert.Equal(t, "selected-use-case", seen.headers.Get("X-Atlassian-UseCaseId"))
			assert.Equal(t, "opaque-session", seen.headers.Get("X-Atlassian-Session-Context"))
			assert.Equal(t, "slauth test", seen.headers.Get("Authorization"))
			assert.Empty(t, seen.headers.Get("X-Api-Key"))

			request := `{"model":"gpt-5.4","input":[{"role":"user","content":"hello"},{"type":"reasoning","encrypted_content":"opaque","summary":[]},{"type":"web_search_call","id":"search_1"}],"store":false,"text":{"verbosity":"low","format":{"type":"text"}},"tools":[{"type":"web_search"},{"type":"function","name":"lookup","defer_loading":true,"parameters":{"type":"object"}}],"prompt_cache_retention":"24h"}`
			res = postGateway(t, base, openAIV2Path, request)
			require.Equal(t, 200, res.StatusCode)
			seen = <-requests
			assert.Equal(t, nativeV2Path, seen.path)
			assert.Equal(t, "fallback-cloud", seen.headers.Get("X-Atlassian-CloudId"))
			got := seen.body
			assert.Equal(t, "gpt-5.4-2026-03-05", got["model"])
			input := list(got["input"])
			assert.Equal(t, "message", object(input[0])["type"])
			assert.Equal(t, "opaque", object(input[1])["encrypted_content"])
			assert.Equal(t, "web_search_call", providerBlock(object(input[2]), "openai")["type"])
			assert.Equal(t, false, got["store"])
			assert.Equal(t, jsonObject{"format": jsonObject{"type": "text"}}, got["text"])
			options := object(object(object(got["provider"])["options"])["openai"])
			assert.Equal(t, "24h", options["prompt_cache_retention"])
			assert.Equal(t, []any{jsonObject{"type": "web_search"}}, options["tools"])
			tool := object(list(got["tools"])[0])
			assert.Equal(t, "lookup", tool["name"])
			assert.Equal(t, true, object(object(object(tool["provider"])["options"])["openai"])["defer_loading"])
			result, err := decodeObject(res.Body)
			require.NoError(t, err)
			assert.Equal(t, "ready", result["output_text"])
			assert.Equal(t, "web_search_call", object(list(result["output"])[0])["type"])

			res = postGateway(t, base, openAIV2Path, `{"model":"gpt-5.4","input":"stream-test","stream":true}`)
			require.Equal(t, 200, res.StatusCode)
			<-requests
			stream, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Contains(t, string(stream), "event: response.completed\n")
			assert.Contains(t, string(stream), "\"output_text\":\"ready\"")

			res = postGateway(t, base, openAIV2Path, `{"model":"gpt-5.4","previous_response_id":"resp_1"}`)
			assert.Equal(t, 400, res.StatusCode)
			assert.Empty(t, requests)
			res = postGateway(t, base, claudeV2Path, `{"model":"force-upstream-error","messages":[{"role":"user","content":"hello"}]}`)
			assert.Equal(t, 400, res.StatusCode)
			<-requests
			result, err = decodeObject(res.Body)
			require.NoError(t, err)
			assert.Equal(t, "error", result["type"])
			assert.Equal(t, "gateway-123", result["request_id"])
			assert.Equal(t, "Invalid model", object(result["error"])["message"])

			for route, upstream := range map[string]string{
				"/openai/v1/responses":       "/v1/openai/v1/responses",
				"/vertex/claude/v1/messages": "/v1/google/v1/publishers/anthropic/models/claude-haiku-4-5@20251001:rawPredict",
			} {
				res := postGateway(t, base, route, `{"model":"claude-haiku-4-5-20251001","messages":[{"role":"user","content":"hello"}]}`)
				require.Equal(t, 200, res.StatusCode)
				assert.Equal(t, upstream, (<-requests).path)
			}
		})
	}
}

func TestClaudeV2StreamsAndReplaysToolAndReasoningHistory(t *testing.T) {
	requests := make(chan gatewayRequest, 2)
	release := make(chan struct{})
	base := gatewayProxy(t, "cmd/commands/ai-gateway/config.yaml", func(w http.ResponseWriter, r *http.Request, req gatewayRequest) {
		requests <- req
		if strings.Contains(req.raw, "tool result") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"msg_2","model":"claude-sonnet-4-6","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":25,"output_tokens":2,"input_tokens_details":{"cached_tokens":5}}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"type":"response.created","response":{"id":"msg_1","model":"claude-sonnet-4-6"}}`,
			`{"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"Need a lookup."}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","summary":[{"type":"reasoning_text","text":"Need a lookup."}],"encrypted_content":"signed-thinking"}}`,
			`{"type":"response.output_item.done","output_index":1,"item":{"type":"reasoning","summary":[],"encrypted_content":"redacted-thinking"}}`,
			`{"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"Checking."}`,
		} {
			fmt.Fprintf(w, "data: %s\r\n\r\n", data)
		}
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		for _, data := range []string{
			`{"type":"response.output_item.done","output_index":2,"item":{"type":"message","content":[{"type":"output_text","text":"Checking."}]}}`,
			`{"type":"response.output_item.added","output_index":3,"item":{"type":"function_call","call_id":"toolu_1","name":"lookup","arguments":""}}`,
			`{"type":"response.function_call_arguments.delta","output_index":3,"delta":"{\"key\":"}`,
			`{"type":"response.function_call_arguments.delta","output_index":3,"delta":"\"A\"}"}`,
			`{"type":"response.output_item.done","output_index":3,"item":{"type":"function_call","call_id":"toolu_1","name":"lookup","arguments":"{\"key\":\"A\"}"}}`,
			`{"type":"response.completed","response":{"id":"msg_1","status":"completed","usage":{"input_tokens":20,"output_tokens":8,"input_tokens_details":{"cached_tokens":6}}}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
	})
	const systemAndTools = `"model":"claude-sonnet-4-6","system":[{"type":"text","text":"Be concise.","cache_control":{"type":"ephemeral"}}],"max_tokens":512,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"key":{"type":"string"}}}}],"tool_choice":{"type":"auto","disable_parallel_tool_use":true}`
	res := postGateway(t, base, claudeV2Path, "{"+systemAndTools+`,"stream":true,"messages":[{"role":"user","content":"check A"}]}`)
	require.Equal(t, 200, res.StatusCode)
	require.Contains(t, res.Header.Get("Content-Type"), "text/event-stream")
	reader := bufio.NewReader(res.Body)
	var beforeRelease strings.Builder
	for !strings.Contains(beforeRelease.String(), "Checking.") {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		beforeRelease.WriteString(line)
	}
	close(release)
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	stream := beforeRelease.String() + string(rest)
	require.Contains(t, stream, "event: message_stop")

	var transcript []jsonObject
	var args strings.Builder
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		event, err := decodeObject(strings.NewReader(strings.TrimPrefix(line, "data: ")))
		require.NoError(t, err)
		switch event["type"] {
		case "content_block_start":
			require.Equal(t, len(transcript), intField(event["index"]))
			transcript = append(transcript, object(event["content_block"]))
		case "content_block_delta":
			block := transcript[intField(event["index"])]
			delta := object(event["delta"])
			for _, field := range []string{"thinking", "signature", "text"} {
				if value := str(delta[field]); value != "" {
					block[field] = str(block[field]) + value
				}
			}
			args.WriteString(str(delta["partial_json"]))
		case "message_delta":
			assert.Equal(t, "tool_use", object(event["delta"])["stop_reason"])
			assert.Equal(t, 14, intField(object(event["usage"])["input_tokens"]))
			assert.Equal(t, 8, intField(object(event["usage"])["output_tokens"]))
		}
	}
	require.Len(t, transcript, 4)
	assert.Equal(t, "Need a lookup.", transcript[0]["thinking"])
	assert.Equal(t, "toolu_1", transcript[3]["id"])
	require.JSONEq(t, `{"key":"A"}`, args.String())
	transcript[3]["input"], err = decodeObject(strings.NewReader(args.String()))
	require.NoError(t, err)
	first := <-requests
	assert.Equal(t, nativeV2Path, first.path)
	assert.Equal(t, "test-beta", first.headers.Get("Anthropic-Beta"))
	assert.Equal(t, jsonObject{"effort": "high", "summary": "concise"}, first.body["reasoning"])
	assert.Equal(t, false, first.body["parallel_tool_calls"])
	system := object(list(first.body["input"])[0])
	assert.Equal(t, "system", system["role"])
	assert.Equal(t, jsonObject{"mode": "explicit"}, object(list(system["content"])[0])["prompt_cache_breakpoint"])

	replayed, err := json.Marshal(transcript)
	require.NoError(t, err)
	turn2 := "{" + systemAndTools + `,"messages":[{"role":"user","content":"check A"},{"role":"assistant","content":` + string(replayed) + `},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"tool result"}]}]}`
	res = postGateway(t, base, claudeV2Path, turn2)
	require.Equal(t, 200, res.StatusCode)
	result, err := decodeObject(res.Body)
	require.NoError(t, err)
	assert.Equal(t, "end_turn", result["stop_reason"])
	assert.Equal(t, "done", object(list(result["content"])[0])["text"])
	input := list((<-requests).body["input"])
	require.Len(t, input, 7)
	assert.Equal(t, "signed-thinking", object(input[2])["encrypted_content"])
	assert.Equal(t, "redacted-thinking", object(input[3])["encrypted_content"])
	assert.Equal(t, "output_text", object(list(object(input[4])["content"])[0])["type"])
	assert.Equal(t, "toolu_1", object(input[5])["call_id"])
	assert.Equal(t, "toolu_1", object(input[6])["call_id"])
	assert.Equal(t, "tool result", object(input[6])["output"])
}

func TestClaudeV2MapsIncompleteAndInterruptedStreams(t *testing.T) {
	base := gatewayProxy(t, "config.yaml", func(w http.ResponseWriter, _ *http.Request, req gatewayRequest) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"msg_1\"}}\n\n")
		if req.body["model"] == "token-limit" {
			fmt.Fprint(w, "data: {\"type\":\"response.incomplete\",\n")
			fmt.Fprint(w, "data: \"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"output_tokens\":24}}}\n\n")
		}
	})
	for _, model := range []string{"token-limit", "interrupted"} {
		t.Run(model, func(t *testing.T) {
			res := postGateway(t, base, claudeV2Path, fmt.Sprintf(`{"model":%q,"stream":true,"messages":[{"role":"user","content":"hello"}]}`, model))
			require.Equal(t, 200, res.StatusCode)
			output, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			if model == "token-limit" {
				assert.Contains(t, string(output), `"stop_reason":"max_tokens"`)
				assert.Contains(t, string(output), "event: message_stop\n")
			} else {
				assert.Contains(t, string(output), "event: error\n")
				assert.NotContains(t, string(output), "event: message_stop\n")
			}
		})
	}
}
