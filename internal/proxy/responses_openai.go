package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var openAIModelAliases = map[string]string{
	"gpt-5.5":      "gpt-5.5-2026-04-23",
	"gpt-5.4":      "gpt-5.4-2026-03-05",
	"gpt-5.4-mini": "gpt-5.4-mini-2026-03-17",
	"gpt-5.4-nano": "gpt-5.4-nano-2026-03-17",
}

func openAIRequest(body jsonObject) (jsonObject, error) {
	switch {
	case body["previous_response_id"] != nil, body["conversation"] != nil:
		return nil, fmt.Errorf("AI Gateway v2 requires the full input history instead of previous_response_id or conversation")
	case body["store"] == true, body["background"] == true:
		return nil, fmt.Errorf("AI Gateway v2 requires store and background to be false or omitted")
	}

	out := fields(body, "model", "input", "instructions", "stream", "include", "tool_choice",
		"parallel_tool_calls", "max_output_tokens", "max_tool_calls", "truncation",
		"reasoning", "temperature", "top_p", "top_logprobs", "presence_penalty",
		"frequency_penalty", "metadata", "user", "prompt_cache_key", "service_tier",
		"provider", "prompt_cache_options", "stream_options")
	if id := openAIModelAliases[str(body["model"])]; id != "" {
		out["model"] = id
	}

	input := list(body["input"])
	for index, entry := range input {
		item := object(entry)
		if item["type"] == "item_reference" {
			return nil, fmt.Errorf("AI Gateway v2 requires full input items instead of item references")
		}
		if item["type"] == nil && item["role"] != nil {
			item["type"] = "message"
		}
		switch item["type"] {
		case "web_search_call", "file_search_call", "code_interpreter_call", "computer_call",
			"computer_call_output", "image_generation_call", "local_shell_call", "local_shell_call_output",
			"shell_call", "shell_call_output", "apply_patch_call", "apply_patch_call_output",
			"mcp_call", "mcp_list_tools", "mcp_approval_request", "mcp_approval_response",
			"custom_tool_call", "custom_tool_call_output":
			block := jsonObject{"type": "platform:provider_block", "provider": "openai", "data": item}
			if id := str(item["id"]); id != "" {
				block["id"] = id
			}
			input[index] = block
		}
	}
	if format := object(body["text"])["format"]; format != nil {
		out["text"] = jsonObject{"format": format}
	}
	options := jsonObject{}
	for name, value := range body {
		switch name {
		case "store", "background", "previous_response_id", "conversation", "tools", "text":
			continue
		}
		if _, canonical := out[name]; !canonical && value != nil {
			options[name] = value
		}
	}
	out["store"] = false
	var functions, hosted []any
	for _, entry := range list(body["tools"]) {
		tool := object(entry)
		if tool == nil {
			return nil, fmt.Errorf("expected a tool object")
		}
		if tool["type"] == "function" {
			moveOpenAIToolOptions(tool)
			functions = append(functions, tool)
		} else {
			hosted = append(hosted, tool)
		}
	}
	if len(functions) > 0 {
		out["tools"] = functions
	}
	if len(hosted) > 0 {
		existing := object(object(object(out["provider"])["options"])["openai"])
		options["tools"] = append(list(existing["tools"]), hosted...)
	}
	if len(options) > 0 {
		setProviderOptions(out, "openai", options)
	}
	return out, nil
}

func openAIResponse(response jsonObject) {
	output := list(response["output"])
	if output == nil {
		return
	}
	var text strings.Builder
	for index, raw := range output {
		item := object(raw)
		if block := providerBlock(item, "openai"); block != nil {
			output[index] = block
		}
		if item["type"] == "message" {
			for _, raw := range list(item["content"]) {
				part := object(raw)
				if part["type"] == "output_text" {
					text.WriteString(str(part["text"]))
				}
			}
		}
	}
	response["output_text"] = text.String()
}

func translateOpenAIStream(source io.Reader, out io.Writer) error {
	terminal := false
	err := readResponsesEvents(source, func(event jsonObject) (bool, error) {
		if event == nil {
			_, err := fmt.Fprint(out, ": heartbeat\n\n")
			return false, err
		}
		openAIResponse(object(event["response"]))
		if block := providerBlock(object(event["item"]), "openai"); block != nil {
			event["item"] = block
		}
		kind := str(event["type"])
		switch kind {
		case "response.completed", "response.incomplete", "response.failed", "error":
			terminal = true
		}
		data, err := json.Marshal(event)
		if err != nil {
			return false, err
		}
		// Event names come from the gateway JSON and must fit on a single SSE line.
		kind = strings.ReplaceAll(strings.ReplaceAll(kind, "\n", ""), "\r", "")
		_, err = fmt.Fprintf(out, "event: %s\ndata: %s\n\n", kind, data)
		return terminal, err
	})
	if !terminal && errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func moveOpenAIToolOptions(tool jsonObject) {
	if value, ok := tool["defer_loading"]; ok {
		delete(tool, "defer_loading")
		setProviderOptions(tool, "openai", jsonObject{"defer_loading": value})
	}
}

func setProviderOptions(body jsonObject, provider string, values jsonObject) {
	config := object(body["provider"])
	if config == nil {
		config = jsonObject{}
		body["provider"] = config
	}
	byProvider := object(config["options"])
	if byProvider == nil {
		byProvider = jsonObject{}
		config["options"] = byProvider
	}
	options := object(byProvider[provider])
	if options == nil {
		options = jsonObject{}
		byProvider[provider] = options
	}
	for key, value := range values {
		options[key] = value
	}
}
