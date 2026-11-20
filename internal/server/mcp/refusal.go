package mcp

import (
	"encoding/json"
	"strings"
)

const refusalCodeUnknown = "tool_error"

func uniformRefusal(content []byte) []byte {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(content, &body); err != nil || body == nil {
		return content
	}
	code := stringField(body, "code")
	if code == "" {
		code = refusalCodeUnknown
		body["code"] = quoted(code)
	}
	if stringField(body, "message") == "" {
		message := stringField(body, "error")
		if message == "" {
			message = "the call was refused with code " + code + "; the other fields say why"
		}
		body["message"] = quoted(message)
	}
	delete(body, "error")
	out, err := json.Marshal(body)
	if err != nil {
		return content
	}
	return out
}

func stringField(body map[string]json.RawMessage, key string) string {
	var s string
	if raw, ok := body[key]; ok && json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func quoted(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}
