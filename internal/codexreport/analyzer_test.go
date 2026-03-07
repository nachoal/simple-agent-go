package codexreport

import (
	"encoding/json"
	"testing"
)

func TestDecodeToolInvocations_ExpandsParallelExecCommands(t *testing.T) {
	raw := json.RawMessage(`{
		"tool_uses": [
			{
				"recipient_name": "functions.exec_command",
				"parameters": {
					"cmd": "go test ./...",
					"workdir": "/tmp/repo"
				}
			},
			{
				"recipient_name": "functions.exec_command",
				"parameters": {
					"cmd": "git status --short",
					"workdir": "/tmp/repo"
				}
			}
		]
	}`)

	got := decodeToolInvocations("multi_tool_use.parallel", raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(got))
	}
	if got[0].Name != "exec_command" || got[0].Command != "go test ./..." || got[0].Workdir != "/tmp/repo" {
		t.Fatalf("unexpected first invocation: %+v", got[0])
	}
	if got[1].Command != "git status --short" {
		t.Fatalf("unexpected second invocation: %+v", got[1])
	}
}

func TestClassifySession_DemotesCrossProjectPathOnlySessions(t *testing.T) {
	acc := sessionAccumulator{
		SessionCwd: "/Users/ia/code/projects/simple-agents/simple-agent-go",
		TurnCwds: map[string]struct{}{
			"/Users/ia/code/projects/simple-agents/simple-agent-go": {},
		},
		EventConversation: []conversationMessage{
			{
				Role: "user",
				Text: "i want to build something in /Users/ia/code/projects/clis using claudito; the go repo is just an example",
			},
		},
	}

	reasons, classification, others := classifySession(acc, "/Users/ia/code/projects/simple-agents/simple-agent-go", acc.EventConversation[0].Text)
	if classification != "path_only" && classification != "cross_project" {
		t.Fatalf("expected path_only or cross_project classification, got %q (%v)", classification, reasons)
	}
	if len(others) == 0 {
		t.Fatalf("expected other project references to be detected")
	}
}

func TestClassifySession_PromotesRepoScopedCommands(t *testing.T) {
	acc := sessionAccumulator{
		SessionCwd: "/Users/ia/code/projects/simple-agents/simple-agent-go",
		TurnCwds: map[string]struct{}{
			"/Users/ia/code/projects/simple-agents/simple-agent-go": {},
		},
		EventConversation: []conversationMessage{
			{Role: "user", Text: "run go test ./... and see why it fails"},
		},
		ToolInvocations: []toolInvocation{
			{Name: "exec_command", Command: "go test ./...", Workdir: "/Users/ia/code/projects/simple-agents/simple-agent-go"},
		},
	}

	_, classification, _ := classifySession(acc, "/Users/ia/code/projects/simple-agents/simple-agent-go", acc.EventConversation[0].Text)
	if classification != "primary" {
		t.Fatalf("expected primary classification, got %q", classification)
	}
}

func TestDetectLoopIncidents_FindsPrematureCompletion(t *testing.T) {
	acc := sessionAccumulator{ID: "s1", RelativeFile: "sessions/example.jsonl"}
	conversation := []conversationMessage{
		{Role: "assistant", Text: "The repo checks are clean. I am done."},
		{Role: "user", Text: "did you test the interactive flow though?"},
		{Role: "assistant", Text: "I am checking that now."},
	}

	got := detectLoopIncidents(acc, conversation, "primary")
	if len(got) != 1 {
		t.Fatalf("expected 1 loop incident, got %d", len(got))
	}
	if got[0].UserCorrection != "did you test the interactive flow though?" {
		t.Fatalf("unexpected correction text: %+v", got[0])
	}
}

func TestApplyLegacyValue_CollectsMessagesAndToolCalls(t *testing.T) {
	raw := json.RawMessage(`{
		"session": {
			"timestamp": "2025-06-11T04:20:52.757Z",
			"id": "legacy-1"
		},
		"items": [
			{
				"type": "message",
				"role": "user",
				"content": [
					{"type": "input_text", "text": "what are the gh issues for this project?"}
				]
			},
			{
				"type": "function_call",
				"name": "shell",
				"arguments": "{\"command\":[\"bash\",\"-lc\",\"gh issue list\"],\"timeout\":10000}"
			}
		]
	}`)

	acc := sessionAccumulator{TurnCwds: map[string]struct{}{}}
	applyLegacyValue(&acc, raw)

	if acc.ID != "legacy-1" {
		t.Fatalf("expected legacy session id to be set, got %q", acc.ID)
	}
	if len(acc.ResponseConversation) != 1 || acc.ResponseConversation[0].Role != "user" {
		t.Fatalf("unexpected legacy response conversation: %+v", acc.ResponseConversation)
	}
	if len(acc.ToolInvocations) != 1 || acc.ToolInvocations[0].Name != "shell" {
		t.Fatalf("unexpected legacy tool invocations: %+v", acc.ToolInvocations)
	}
}
