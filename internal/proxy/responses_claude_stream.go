package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type claudeBlockKey struct {
	output, content int
}

type claudeStreamBlock struct {
	index, sent int
	closed      bool
}

type claudeStream struct {
	writer         io.Writer
	model          string
	started, ended bool
	hasTool        bool
	blocks         map[claudeBlockKey]*claudeStreamBlock
	err            error
}

func (s *server) translateClaudeStream(res *http.Response) {
	model := res.Header.Get("X-Atlassian-Serving-Model")
	convertResponseStream(res, func(source io.Reader, out io.Writer) error {
		stream := claudeStream{writer: out, model: model, blocks: make(map[claudeBlockKey]*claudeStreamBlock)}
		err := readResponsesEvents(source, func(event jsonObject) (bool, error) {
			if event == nil {
				stream.emit("ping", jsonObject{})
			} else {
				stream.event(event)
			}
			return stream.ended, stream.err
		})
		if !stream.ended && stream.err == nil && !errors.Is(err, io.ErrClosedPipe) {
			s.Logger.Printf("Claude v2 stream: %v", err)
			stream.emit("error", responseError(claudeMessages, http.StatusBadGateway, "AI Gateway stream ended without a complete message", "", ""))
		}
		return stream.err
	})
}

func (s *claudeStream) emit(kind string, fields jsonObject) {
	if s.err != nil {
		return
	}
	fields["type"] = kind
	data, err := json.Marshal(fields)
	if err != nil {
		s.err = err
		return
	}
	_, s.err = fmt.Fprintf(s.writer, "event: %s\ndata: %s\n\n", kind, data)
}

func (s *claudeStream) startMessage(response jsonObject) {
	if s.started {
		return
	}
	message := claudeMessage(response)
	if message["model"] == "" {
		message["model"] = s.model
	}
	s.emit("message_start", jsonObject{"message": message})
	s.started = true
}

func (s *claudeStream) startBlock(key claudeBlockKey, content jsonObject) *claudeStreamBlock {
	block := s.blocks[key]
	if block == nil {
		block = &claudeStreamBlock{index: len(s.blocks)}
		s.blocks[key] = block
		s.emit("content_block_start", jsonObject{"index": block.index, "content_block": content})
	}
	return block
}

func (s *claudeStream) delta(block *claudeStreamBlock, kind, field, value string) {
	if value != "" && !block.closed {
		s.emit("content_block_delta", jsonObject{
			"index": block.index, "delta": jsonObject{"type": kind, field: value},
		})
		block.sent += len(value)
	}
}

func (s *claudeStream) stopBlock(block *claudeStreamBlock) {
	if !block.closed {
		s.emit("content_block_stop", jsonObject{"index": block.index})
		block.closed = true
	}
}

func (s *claudeStream) startTool(key claudeBlockKey, item jsonObject) *claudeStreamBlock {
	s.hasTool = true
	return s.startBlock(key, jsonObject{"type": "tool_use", "id": item["call_id"], "name": item["name"], "input": jsonObject{}})
}

func (s *claudeStream) startText(key claudeBlockKey) *claudeStreamBlock {
	return s.startBlock(key, jsonObject{"type": "text", "text": ""})
}

func (s *claudeStream) startThinking(key claudeBlockKey) *claudeStreamBlock {
	return s.startBlock(key, jsonObject{"type": "thinking", "thinking": "", "signature": ""})
}

func (s *claudeStream) finishText(key claudeBlockKey, part jsonObject) {
	if text, ok := claudeOutputText(part); ok {
		block := s.startText(key)
		s.delta(block, "text_delta", "text", text[min(len(text), block.sent):])
		s.stopBlock(block)
	}
}

func (s *claudeStream) finishItem(index int, item jsonObject) {
	key := claudeBlockKey{output: index}
	switch item["type"] {
	case "message":
		for i, raw := range list(item["content"]) {
			s.finishText(claudeBlockKey{index, i}, object(raw))
		}
	case "function_call":
		block := s.startTool(key, item)
		args := str(item["arguments"])
		s.delta(block, "input_json_delta", "partial_json", args[min(len(args), block.sent):])
		s.stopBlock(block)
	case "reasoning":
		signature, text := str(item["encrypted_content"]), reasoningText(item)
		block := s.blocks[key]
		switch {
		case block != nil || text != "":
			block = s.startThinking(key)
			s.delta(block, "thinking_delta", "thinking", text[min(len(text), block.sent):])
			s.delta(block, "signature_delta", "signature", signature)
			s.stopBlock(block)
		case signature != "":
			block = s.startBlock(key, jsonObject{"type": "redacted_thinking", "data": signature})
			s.stopBlock(block)
		}
	case "platform:provider_block":
		if content := providerBlock(item, "anthropic"); content != nil {
			s.stopBlock(s.startBlock(key, content))
		}
	}
}

func (s *claudeStream) event(event jsonObject) {
	key := claudeBlockKey{intField(event["output_index"]), intField(event["content_index"])}
	item, response := object(event["item"]), object(event["response"])
	value := str(event["delta"])
	switch event["type"] {
	case "response.created", "response.in_progress":
		s.startMessage(response)
	case "response.output_item.added":
		if item["type"] == "function_call" {
			block := s.startTool(key, item)
			s.delta(block, "input_json_delta", "partial_json", str(item["arguments"]))
		}
	case "response.content_part.added":
		if text, ok := claudeOutputText(object(event["part"])); ok {
			s.delta(s.startText(key), "text_delta", "text", text)
		}
	case "response.output_text.delta", "response.refusal.delta":
		s.delta(s.startText(key), "text_delta", "text", value)
	case "response.function_call_arguments.delta":
		if block := s.blocks[key]; block != nil {
			s.delta(block, "input_json_delta", "partial_json", value)
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		s.delta(s.startThinking(claudeBlockKey{output: key.output}), "thinking_delta", "thinking", value)
	case "response.content_part.done":
		s.finishText(key, object(event["part"]))
	case "response.output_item.done":
		s.finishItem(key.output, item)
	case "response.completed", "response.incomplete":
		s.startMessage(response)
		if response == nil {
			response = jsonObject{}
		}
		if event["type"] == "response.incomplete" {
			response["status"] = "incomplete"
		}
		for i, raw := range list(response["output"]) {
			s.finishItem(i, object(raw))
		}
		for _, block := range s.blocks {
			s.stopBlock(block)
		}
		if attrs := event["platform_attributes"]; attrs != nil {
			response["platform_attributes"] = attrs
		}
		s.emit("message_delta", jsonObject{
			"delta": jsonObject{"stop_reason": claudeStopReason(response, s.hasTool), "stop_sequence": nil},
			"usage": claudeUsage(response),
		})
		s.emit("message_stop", jsonObject{})
		s.ended = true
	case "error", "response.failed":
		failure := object(response["error"])
		if failure == nil {
			failure = event
		}
		s.emit("error", adaptGatewayError(claudeMessages, http.StatusBadGateway, failure))
		s.ended = true
	}
}
