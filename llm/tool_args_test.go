package llm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToolArguments_Object(t *testing.T) {
	args, normalized := NormalizeToolArguments(json.RawMessage(`{"command":"date","timeout":30}`))

	if args["command"] != "date" {
		t.Fatalf("expected command=date, got %v", args["command"])
	}
	if _, ok := args["timeout"]; !ok {
		t.Fatalf("expected timeout key in args")
	}

	var check map[string]interface{}
	if err := json.Unmarshal(normalized, &check); err != nil {
		t.Fatalf("normalized args is not valid JSON object: %v", err)
	}
	if check["command"] != "date" {
		t.Fatalf("expected normalized command=date, got %v", check["command"])
	}
}

func TestNormalizeToolArguments_QuotedJSONObject(t *testing.T) {
	raw := json.RawMessage(`"{\"command\":\"date\"}"`)
	args, normalized := NormalizeToolArguments(raw)

	if args["command"] != "date" {
		t.Fatalf("expected parsed command=date, got %v", args["command"])
	}

	if string(normalized) != `{"command":"date"}` {
		t.Fatalf("unexpected normalized value: %s", string(normalized))
	}
}

func TestNormalizeToolArguments_InvalidReturnsEmptyObject(t *testing.T) {
	args, normalized := NormalizeToolArguments(json.RawMessage(`not-json`))

	if len(args) != 0 {
		t.Fatalf("expected empty args for invalid input, got %v", args)
	}
	if string(normalized) != "{}" {
		t.Fatalf("expected normalized {} for invalid input, got %s", string(normalized))
	}
}

func TestNormalizeToolArguments_NonObjectReturnsEmptyObject(t *testing.T) {
	args, normalized := NormalizeToolArguments(json.RawMessage(`["not","an","object"]`))

	if len(args) != 0 {
		t.Fatalf("expected empty args for non-object input, got %v", args)
	}
	if string(normalized) != "{}" {
		t.Fatalf("expected normalized {} for non-object input, got %s", string(normalized))
	}
}
