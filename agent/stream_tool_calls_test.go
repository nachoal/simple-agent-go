package agent

import (
	"encoding/json"
	"testing"

	"github.com/nachoal/simple-agent-go/llm"
)

func TestMergeStreamToolCallDeltas_ReconstructsSplitArguments(t *testing.T) {
	deltas := []llm.ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "bash",
				Arguments: json.RawMessage(`"{\"command\":\"da"`),
			},
		},
		{
			Function: llm.FunctionCall{
				Arguments: json.RawMessage(`"te\"}"`),
			},
		},
	}

	states := mergeStreamToolCallDeltas(nil, deltas)
	calls := toLLMToolCallsFromStream(states)

	if len(calls) != 1 {
		t.Fatalf("expected 1 merged call, got %d", len(calls))
	}
	if calls[0].ID != "call_1" {
		t.Fatalf("expected ID call_1, got %q", calls[0].ID)
	}
	if calls[0].Function.Name != "bash" {
		t.Fatalf("expected function name bash, got %q", calls[0].Function.Name)
	}

	args, normalized := llm.NormalizeToolArguments(calls[0].Function.Arguments)
	if args["command"] != "date" {
		t.Fatalf("expected command=date, got %v", args["command"])
	}
	if string(normalized) != `{"command":"date"}` {
		t.Fatalf("unexpected normalized args: %s", string(normalized))
	}
}

func TestMergeStreamToolCallDeltas_DropsNamelessCalls(t *testing.T) {
	deltas := []llm.ToolCall{
		{
			Function: llm.FunctionCall{
				Arguments: json.RawMessage(`"{\"command\":\"date\"}"`),
			},
		},
	}

	states := mergeStreamToolCallDeltas(nil, deltas)
	calls := toLLMToolCallsFromStream(states)
	if len(calls) != 0 {
		t.Fatalf("expected no calls for nameless delta, got %d", len(calls))
	}
}

func TestMergeStreamToolCallDeltas_GeneratesIDWhenMissing(t *testing.T) {
	deltas := []llm.ToolCall{
		{
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "bash",
				Arguments: json.RawMessage(`"{\"command\":\"date\"}"`),
			},
		},
	}

	states := mergeStreamToolCallDeltas(nil, deltas)
	calls := toLLMToolCallsFromStream(states)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].ID == "" {
		t.Fatalf("expected generated tool call ID")
	}
}

func TestMergeStreamToolCallDeltas_MergesByNameWhenIDMissing(t *testing.T) {
	deltas := []llm.ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "bash",
				Arguments: json.RawMessage(`"{\"command\":\"da"`),
			},
		},
		{
			Function: llm.FunctionCall{
				Name:      "bash",
				Arguments: json.RawMessage(`"te\"}"`),
			},
		},
	}

	states := mergeStreamToolCallDeltas(nil, deltas)
	calls := toLLMToolCallsFromStream(states)

	if len(calls) != 1 {
		t.Fatalf("expected 1 merged call, got %d", len(calls))
	}
	if calls[0].ID != "call_1" {
		t.Fatalf("expected ID call_1, got %q", calls[0].ID)
	}
	if calls[0].Function.Name != "bash" {
		t.Fatalf("expected function name bash, got %q", calls[0].Function.Name)
	}
	args, normalized := llm.NormalizeToolArguments(calls[0].Function.Arguments)
	if args["command"] != "date" {
		t.Fatalf("expected command=date, got %v", args["command"])
	}
	if string(normalized) != `{"command":"date"}` {
		t.Fatalf("unexpected normalized args: %s", string(normalized))
	}
}

func TestMergeStreamToolCallDeltas_PromotesUnnamedPlaceholder(t *testing.T) {
	deltas := []llm.ToolCall{
		{
			Function: llm.FunctionCall{
				Arguments: json.RawMessage(`"{\"command\":\"da"`),
			},
		},
		{
			ID:   "call_1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "bash",
				Arguments: json.RawMessage(`"te\"}"`),
			},
		},
	}

	states := mergeStreamToolCallDeltas(nil, deltas)
	calls := toLLMToolCallsFromStream(states)

	if len(calls) != 1 {
		t.Fatalf("expected 1 merged call, got %d", len(calls))
	}
	if calls[0].ID != "call_1" {
		t.Fatalf("expected ID call_1, got %q", calls[0].ID)
	}
	if calls[0].Function.Name != "bash" {
		t.Fatalf("expected function name bash, got %q", calls[0].Function.Name)
	}
	args, normalized := llm.NormalizeToolArguments(calls[0].Function.Arguments)
	if args["command"] != "date" {
		t.Fatalf("expected command=date, got %v", args["command"])
	}
	if string(normalized) != `{"command":"date"}` {
		t.Fatalf("unexpected normalized args: %s", string(normalized))
	}
}
