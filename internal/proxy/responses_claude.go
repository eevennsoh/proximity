package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var claudeVertexDate = regexp.MustCompile(`^(.+)-(\d{8})(-v\d+)?$`)

func claudeRequest(body jsonObject) (jsonObject, error) {
	model := str(body["model"])
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if !strings.Contains(model, "/") {
		model = claudeVertexDate.ReplaceAllString(model, "${1}${3}@${2}")
	}
	out := fields(body, "stream", "temperature", "top_p")
	out["model"] = model
	if budget := body["max_tokens"]; budget != nil {
		out["max_output_tokens"] = budget
	}

	turns := list(body["messages"])
	if len(turns) == 0 {
		return nil, fmt.Errorf("messages must contain at least one message")
	}
	input := make([]any, 0, len(turns)+1)
	switch system := body["system"].(type) {
	case string:
		out["instructions"] = system
	case []any:
		items, err := claudeInput("system", system)
		if err != nil {
			return nil, err
		}
		input = append(input, items...)
	}
	for _, turn := range turns {
		message := object(turn)
		role := str(message["role"])
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("message role must be user or assistant")
		}
		blocks := list(message["content"])
		if content, ok := message["content"].(string); ok {
			blocks = []any{jsonObject{"type": "text", "text": content}}
		}
		if blocks == nil {
			return nil, fmt.Errorf("message content must be text or an array")
		}
		items, err := claudeInput(role, blocks)
		if err != nil {
			return nil, err
		}
		input = append(input, items...)
	}
	out["input"] = input

	if err := claudeTools(body, out); err != nil {
		return nil, err
	}
	outputConfig := object(body["output_config"])
	if format := object(outputConfig["format"]); format["type"] == "json_schema" {
		out["text"] = jsonObject{"format": jsonObject{
			"type": "json_schema", "name": "response", "schema": format["schema"], "strict": true,
		}}
	}
	if reasoning := claudeReasoning(object(body["thinking"]), outputConfig); reasoning != nil {
		out["reasoning"] = reasoning
		out["include"] = []string{"reasoning.encrypted_content"}
	}
	if beta := body["anthropic_beta"]; beta != nil {
		setProviderOptions(out, "anthropic", jsonObject{"anthropic_beta": beta})
	}
	return out, nil
}

func claudeInput(role string, blocks []any) ([]any, error) {
	var items, content []any
	flush := func() {
		if len(content) > 0 {
			items = append(items, jsonObject{"type": "message", "role": role, "content": content})
			content = nil
		}
	}
	for _, raw := range blocks {
		block := object(raw)
		var item jsonObject
		switch str(block["type"]) {
		case "tool_use":
			if role != "assistant" || str(block["id"]) == "" || str(block["name"]) == "" || object(block["input"]) == nil {
				return nil, fmt.Errorf("tool_use requires an assistant message, id, name and input object")
			}
			args, err := json.Marshal(block["input"])
			if err != nil {
				return nil, err
			}
			item = jsonObject{"type": "function_call", "call_id": block["id"], "name": block["name"], "arguments": string(args)}
		case "tool_result":
			if role != "user" || str(block["tool_use_id"]) == "" {
				return nil, fmt.Errorf("tool_result requires a user message and tool_use_id")
			}
			result, err := claudeToolResult(block)
			if err != nil {
				return nil, err
			}
			item = jsonObject{"type": "function_call_output", "call_id": block["tool_use_id"], "output": result}
			claudeCacheBreakpoint(item, block)
		case "thinking", "redacted_thinking":
			if role != "assistant" {
				return nil, fmt.Errorf("thinking blocks require an assistant message")
			}
			encrypted := str(block["signature"])
			summary := []any{}
			if block["type"] == "redacted_thinking" {
				encrypted = str(block["data"])
			} else if thinking := str(block["thinking"]); thinking != "" {
				summary = append(summary, jsonObject{"type": "reasoning_text", "text": thinking})
			}
			if encrypted != "" {
				item = jsonObject{"type": "reasoning", "summary": summary, "encrypted_content": encrypted}
			}
		case "server_tool_use", "web_search_tool_result", "web_fetch_tool_result", "code_execution_tool_result",
			"bash_code_execution_tool_result", "text_editor_code_execution_tool_result", "tool_search_tool_result",
			"mcp_tool_use", "mcp_tool_result", "container_upload", "compaction":
			if role != "assistant" {
				return nil, fmt.Errorf("%s blocks require an assistant message", str(block["type"]))
			}
			item = jsonObject{"type": "platform:provider_block", "provider": "anthropic", "data": block}
			if id := str(block["id"]); id != "" {
				item["id"] = id
			}
		default:
			part, err := claudeInputPart(role, block)
			if err != nil {
				return nil, err
			}
			content = append(content, part)
		}
		if item != nil {
			flush()
			items = append(items, item)
		}
	}
	flush()
	return items, nil
}

func claudeInputPart(role string, block jsonObject) (jsonObject, error) {
	var part jsonObject
	switch block["type"] {
	case "text":
		text, ok := block["text"].(string)
		if !ok {
			return nil, fmt.Errorf("text block requires text")
		}
		kind := "input_text"
		if role == "assistant" {
			kind = "output_text"
		}
		part = jsonObject{"type": kind, "text": text}
		if role == "assistant" {
			part["annotations"] = []any{}
		}
	case "image", "document":
		source := object(block["source"])
		mediaType, data := str(source["media_type"]), str(source["data"])
		if role == "assistant" || source["type"] != "base64" || mediaType == "" || data == "" {
			return nil, fmt.Errorf("%s input requires a base64 source", str(block["type"]))
		}
		url := "data:" + mediaType + ";base64," + data
		if block["type"] == "image" {
			part = jsonObject{"type": "input_image", "image_url": url, "detail": "auto"}
		} else {
			filename := str(block["title"])
			if filename == "" {
				filename = "document.pdf"
			}
			part = jsonObject{"type": "input_file", "file_data": url, "filename": filename}
		}
	default:
		return nil, fmt.Errorf("unsupported Claude content block %q", str(block["type"]))
	}
	claudeCacheBreakpoint(part, block)
	return part, nil
}

func claudeCacheBreakpoint(part, block jsonObject) {
	if cache := object(block["cache_control"]); cache["type"] == "ephemeral" {
		part["prompt_cache_breakpoint"] = jsonObject{"mode": "explicit"}
	}
}

func claudeToolResult(block jsonObject) (any, error) {
	failed, _ := block["is_error"].(bool)
	const failedPrefix = "Tool execution failed.\n"
	switch content := block["content"].(type) {
	case nil, string:
		text := str(content)
		if failed {
			text = failedPrefix + text
		}
		return text, nil
	case []any:
		parts := make([]any, 0, len(content)+1)
		if failed {
			parts = append(parts, jsonObject{"type": "input_text", "text": failedPrefix})
		}
		for _, raw := range content {
			part, err := claudeInputPart("user", object(raw))
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		return parts, nil
	default:
		return nil, fmt.Errorf("tool result content must be text or an array")
	}
}

func claudeTools(body, out jsonObject) error {
	var functions, hosted []any
	for _, entry := range list(body["tools"]) {
		tool := object(entry)
		if str(tool["name"]) == "" {
			return fmt.Errorf("tool requires a name")
		}
		if kind := str(tool["type"]); kind != "" && kind != "custom" {
			hosted = append(hosted, tool)
			continue
		}
		fn := fields(tool, "name", "description")
		fn["type"] = "function"
		fn["parameters"] = tool["input_schema"]
		if object(fn["parameters"]) == nil {
			return fmt.Errorf("tool %q requires an input_schema object", tool["name"])
		}
		if options := fields(tool, "cache_control", "defer_loading", "allowed_callers", "input_examples", "strict"); len(options) > 0 {
			setProviderOptions(fn, "anthropic", options)
		}
		functions = append(functions, fn)
	}
	if len(functions) > 0 {
		out["tools"] = functions
	}
	if len(hosted) > 0 {
		setProviderOptions(out, "anthropic", jsonObject{"tools": hosted})
	}
	if choice := object(body["tool_choice"]); choice != nil {
		switch choice["type"] {
		case "auto", "none":
			out["tool_choice"] = choice["type"]
		case "any":
			out["tool_choice"] = "required"
		case "tool":
			out["tool_choice"] = jsonObject{"type": "function", "name": choice["name"]}
		default:
			return fmt.Errorf("unsupported Claude tool_choice %q", str(choice["type"]))
		}
		if v, ok := choice["disable_parallel_tool_use"].(bool); ok {
			out["parallel_tool_calls"] = !v
		}
	}
	return nil
}

func claudeReasoning(thinking, outputConfig jsonObject) jsonObject {
	effort := str(outputConfig["effort"])
	switch thinking["type"] {
	case "enabled":
		if effort == "" {
			effort = "medium"
			budget := intField(thinking["budget_tokens"])
			for _, tier := range []struct {
				tokens int
				effort string
			}{{1024, "minimal"}, {2048, "low"}, {8192, "medium"}, {16384, "high"}, {24576, "xhigh"}, {32768, "max"}} {
				effort = tier.effort
				if budget <= tier.tokens {
					break
				}
			}
		}
	case "adaptive":
		if effort == "" {
			effort = "medium"
		}
	case "disabled":
		return nil
	default:
		if effort == "" {
			return nil
		}
	}
	summary := "concise"
	if thinking["display"] == "omitted" {
		summary = "none"
	}
	return jsonObject{"effort": effort, "summary": summary}
}

func intField(value any) int {
	number, _ := value.(json.Number)
	v, _ := number.Int64()
	return int(v)
}
