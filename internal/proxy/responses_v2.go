package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
)

const (
	openResponses   = "open-responses"
	openAIResponses = "openai-responses"
	claudeMessages  = "claude-messages"
)

type jsonObject = map[string]any

func decodeObject(r io.Reader) (jsonObject, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var body jsonObject
	if err := decoder.Decode(&body); err != nil || body == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("expected a single JSON object")
	}
	return body, nil
}

func object(value any) jsonObject {
	v, _ := value.(jsonObject)
	return v
}

func list(value any) []any {
	v, _ := value.([]any)
	return v
}

func str(value any) string {
	v, _ := value.(string)
	return v
}

func fields(body jsonObject, keys ...string) jsonObject {
	result := make(jsonObject, len(keys))
	for _, key := range keys {
		if value, ok := body[key]; ok {
			result[key] = value
		}
	}
	return result
}

func providerBlock(item jsonObject, provider string) jsonObject {
	if item["type"] == "platform:provider_block" && item["provider"] == provider {
		return object(item["data"])
	}
	return nil
}

func (s *server) prepareResponsesV2(r *http.Request, protocol string) (http.Header, error) {
	headers := make(http.Header)
	for _, name := range []string{"X-Atlassian-Session-Context", "Anthropic-Beta"} {
		if values := r.Header.Values(name); len(values) > 0 {
			headers[name] = values
		}
	}
	headers.Set("Accept", "application/json, text/event-stream")
	if protocol == openResponses {
		if accept := r.Header.Get("Accept"); accept != "" {
			headers.Set("Accept", accept)
		}
		return headers, nil
	}

	body, err := decodeObject(r.Body)
	if err != nil {
		return nil, err
	}
	switch protocol {
	case openAIResponses:
		body, err = openAIRequest(body)
	case claudeMessages:
		body, err = claudeRequest(body)
	}
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	r.Body.Close()
	s.applyNewBodyToRequest(r, data)
	return headers, nil
}

func (s *server) modifyResponsesV2(res *http.Response, protocol string) error {
	if protocol == openResponses {
		return nil
	}
	if protocol == claudeMessages {
		res.Header.Set("Access-Control-Allow-Origin", "*")
	}
	mediaType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if res.StatusCode < 400 && mediaType == "text/event-stream" {
		if protocol == claudeMessages {
			s.translateClaudeStream(res)
		} else {
			convertResponseStream(res, translateOpenAIStream)
		}
		return nil
	}

	data, err := copyBody(&res.Body)
	if err != nil {
		return err
	}
	body, err := decodeObject(bytes.NewReader(data))
	switch {
	case res.StatusCode >= 400:
		if id := str(body["requestId"]); id != "" && res.Header.Get("Request-Id") == "" {
			res.Header.Set("Request-Id", id)
		}
		body = adaptGatewayError(protocol, res.StatusCode, body)
	case err != nil:
		return err
	case protocol == claudeMessages:
		body, err = claudeResponse(body)
		if err != nil {
			return err
		}
	case protocol == openAIResponses:
		openAIResponse(body)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	res.Header.Set("Content-Type", "application/json")
	res.Header.Del("Content-Encoding")
	s.applyNewBodyToResponse(res, encoded)
	return nil
}

func responseError(protocol string, status int, message, code, requestID string) jsonObject {
	kind := "api_error"
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = "invalid_request_error"
	case http.StatusUnauthorized:
		kind = "authentication_error"
	case http.StatusForbidden:
		kind = "permission_error"
	case http.StatusNotFound:
		kind = "not_found_error"
	case http.StatusRequestEntityTooLarge:
		kind = "request_too_large"
	case http.StatusTooManyRequests:
		kind = "rate_limit_error"
	case http.StatusServiceUnavailable, 529:
		kind = "overloaded_error"
	}
	out := jsonObject{"error": jsonObject{"message": message, "type": kind}}
	if protocol == claudeMessages {
		out["type"] = "error"
		if requestID != "" {
			out["request_id"] = requestID
		}
	} else if code != "" {
		object(out["error"])["code"] = code
	}
	return out
}

func adaptGatewayError(protocol string, status int, body jsonObject) jsonObject {
	upstream := object(object(object(body["upstream"])["content"])["error"])
	nested := object(body["error"])
	requestID := str(body["requestId"])
	if requestID == "" {
		requestID = str(body["request_id"])
	}
	for _, message := range []string{str(body["message"]), str(upstream["message"]), str(nested["message"]), http.StatusText(status)} {
		if message != "" {
			return responseError(protocol, status, message, str(body["code"]), requestID)
		}
	}
	return responseError(protocol, status, "Gateway request failed", "", requestID)
}

func writeResponsesRequestError(w http.ResponseWriter, protocol string, err error) {
	writeResponsesError(w, protocol, http.StatusBadRequest, err.Error())
}

func writeResponsesError(w http.ResponseWriter, protocol string, status int, message string) {
	if protocol == claudeMessages {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(responseError(protocol, status, message, "", ""))
}
