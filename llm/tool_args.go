package llm

import (
	"bytes"
	"encoding/json"
)

var emptyToolArgs = json.RawMessage(`{}`)

// NormalizeToolArguments converts raw tool arguments into a canonical JSON object.
// It accepts either a JSON object or a JSON-encoded string containing an object.
// Invalid/non-object values are normalized to an empty object.
func NormalizeToolArguments(raw json.RawMessage) (map[string]interface{}, json.RawMessage) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return map[string]interface{}{}, emptyToolArgs
	}

	// Some providers return tool args as a JSON string. Unquote once first.
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var unquoted string
		if err := json.Unmarshal(trimmed, &unquoted); err != nil {
			return map[string]interface{}{}, emptyToolArgs
		}
		trimmed = bytes.TrimSpace([]byte(unquoted))
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return map[string]interface{}{}, emptyToolArgs
		}
	}

	var v interface{}
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return map[string]interface{}{}, emptyToolArgs
	}

	args, ok := v.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}, emptyToolArgs
	}

	normalized, err := json.Marshal(args)
	if err != nil || len(normalized) == 0 {
		return map[string]interface{}{}, emptyToolArgs
	}

	return args, json.RawMessage(normalized)
}
