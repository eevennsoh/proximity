package proxy

import (
	"fmt"
	"strings"
)

func claudeResponse(response jsonObject) (jsonObject, error) {
	if response["status"] == "failed" {
		return nil, fmt.Errorf("gateway returned a failed response")
	}
	out := claudeMessage(response)
	content := make([]any, 0, len(list(response["output"])))
	hasTool := false
	for _, raw := range list(response["output"]) {
		item := object(raw)
		switch item["type"] {
		case "message":
			for _, part := range list(item["content"]) {
				if text, ok := claudeOutputText(object(part)); ok {
					content = append(content, jsonObject{"type": "text", "text": text})
				}
			}
		case "function_call":
			input, err := decodeObject(strings.NewReader(str(item["arguments"])))
			if err != nil {
				return nil, fmt.Errorf("gateway returned invalid function arguments")
			}
			content = append(content, jsonObject{
				"type": "tool_use", "id": item["call_id"], "name": item["name"], "input": input,
			})
			hasTool = true
		case "reasoning":
			if block := claudeThinkingBlock(item); block != nil {
				content = append(content, block)
			}
		case "platform:provider_block":
			if block := providerBlock(item, "anthropic"); block != nil {
				content = append(content, block)
			}
		}
	}
	out["content"] = content
	out["stop_reason"] = claudeStopReason(response, hasTool)
	return out, nil
}

func claudeMessage(response jsonObject) jsonObject {
	return jsonObject{
		"type": "message", "role": "assistant", "id": str(response["id"]), "model": str(response["model"]),
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": claudeUsage(response),
	}
}

func claudeUsage(response jsonObject) jsonObject {
	usage := object(response["usage"])
	platform := object(object(response["platform_attributes"])["usage"])
	cached := intField(object(usage["input_tokens_details"])["cached_tokens"])
	created := intField(platform["cache_creation_input_tokens"])
	input := max(0, intField(usage["input_tokens"])-cached-created)
	if n, ok := platform["uncached_input_tokens"]; ok {
		input = intField(n)
	}
	return jsonObject{
		"input_tokens": input, "output_tokens": intField(usage["output_tokens"]),
		"cache_read_input_tokens": cached, "cache_creation_input_tokens": created,
	}
}

func claudeStopReason(response jsonObject, tool bool) string {
	if response["status"] == "incomplete" {
		switch object(response["incomplete_details"])["reason"] {
		case "max_output_tokens":
			return "max_tokens"
		case "content_filter":
			return "refusal"
		default:
			return "pause_turn"
		}
	}
	if tool {
		return "tool_use"
	}
	return "end_turn"
}

func claudeOutputText(part jsonObject) (string, bool) {
	switch part["type"] {
	case "output_text":
		return str(part["text"]), true
	case "refusal":
		return str(part["refusal"]), true
	default:
		return "", false
	}
}

func reasoningText(item jsonObject) string {
	var text strings.Builder
	for _, part := range list(item["summary"]) {
		text.WriteString(str(object(part)["text"]))
	}
	return text.String()
}

func claudeThinkingBlock(item jsonObject) jsonObject {
	text, signature := reasoningText(item), str(item["encrypted_content"])
	switch {
	case text != "":
		return jsonObject{"type": "thinking", "thinking": text, "signature": signature}
	case signature != "":
		return jsonObject{"type": "redacted_thinking", "data": signature}
	default:
		return nil
	}
}
