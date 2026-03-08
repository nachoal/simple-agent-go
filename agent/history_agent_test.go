package agent

import (
	"context"
	"testing"

	"github.com/nachoal/simple-agent-go/history"
	"github.com/nachoal/simple-agent-go/llm"
)

type preservingStubAgent struct {
	memory []llm.Message
}

func (a *preservingStubAgent) Query(context.Context, string) (*Response, error) {
	return nil, nil
}

func (a *preservingStubAgent) QueryStream(context.Context, string) (<-chan StreamEvent, error) {
	user := "follow up"
	reply := "visible assistant reply"
	a.memory = append(a.memory,
		llm.Message{Role: llm.RoleUser, Content: llm.StringPtr(user)},
		llm.Message{Role: llm.RoleAssistant, Content: llm.StringPtr(reply)},
	)

	ch := make(chan StreamEvent, 1)
	go func() {
		defer close(ch)
		ch <- StreamEvent{Type: EventTypeError, Error: context.Canceled}
	}()
	return ch, nil
}

func (a *preservingStubAgent) Clear() {
	a.memory = nil
}

func (a *preservingStubAgent) GetMemory() []llm.Message {
	out := make([]llm.Message, len(a.memory))
	copy(out, a.memory)
	return out
}

func (a *preservingStubAgent) SetSystemPrompt(string) {}

func (a *preservingStubAgent) SetMemory(messages []llm.Message) {
	a.memory = make([]llm.Message, len(messages))
	copy(a.memory, messages)
}

func (a *preservingStubAgent) SetRequestParams(RequestParams) {}

func (a *preservingStubAgent) GetRequestParams() RequestParams { return RequestParams{} }

func TestHistoryAgentQueryStream_PreservesCommittedTurnOnCancel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mgr, err := history.NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	session, err := mgr.StartSession("/tmp/project", "openai", "gpt-4")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	baseMemory := []llm.Message{
		{Role: llm.RoleSystem, Content: llm.StringPtr("system")},
		{Role: llm.RoleUser, Content: llm.StringPtr("hello")},
		{Role: llm.RoleAssistant, Content: llm.StringPtr("hi")},
	}
	session.Messages = mgr.ConvertFromLLMMessages(baseMemory)
	if err := mgr.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	underlying := &preservingStubAgent{memory: baseMemory}
	ha := NewHistoryAgent(underlying, mgr, session)

	stream, err := ha.QueryStream(context.Background(), "follow up")
	if err != nil {
		t.Fatalf("QueryStream: %v", err)
	}
	for range stream {
	}

	loaded, err := mgr.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(loaded.Messages) != len(baseMemory)+2 {
		t.Fatalf("expected preserved committed turn, got %d messages", len(loaded.Messages))
	}
	if loaded.Metadata.LastRunStatus != history.RunStatusCancelled {
		t.Fatalf("expected cancelled run status, got %q", loaded.Metadata.LastRunStatus)
	}
	if loaded.Messages[len(loaded.Messages)-1].Content == nil || *loaded.Messages[len(loaded.Messages)-1].Content != "visible assistant reply" {
		t.Fatalf("expected assistant reply to persist, got %+v", loaded.Messages[len(loaded.Messages)-1])
	}
}

func TestHistoryAgentRestoreMemoryFromSessionUsesVisibleTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mgr, err := history.NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	session, err := mgr.StartSession("/tmp/project", "openai", "gpt-4")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	system := "system"
	user := "who is she?"
	toolOutput := "Found search results"
	assistant := "She is a surgeon in Monterrey."
	session.Messages = []history.Message{
		{Role: "system", Content: &system},
		{Role: "user", Content: &user},
		{
			Role:    "assistant",
			Content: llm.StringPtr(""),
			ToolCalls: []history.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: history.FunctionCall{
					Name:      "google_search",
					Arguments: `{"query":"ximena"}`,
				},
			}},
		},
		{Role: "tool", Content: &toolOutput, ToolCallID: "call-1"},
		{Role: "assistant", Content: &assistant},
	}

	underlying := &preservingStubAgent{}
	ha := NewHistoryAgent(underlying, mgr, session)
	ha.RestoreMemoryFromSession(session)

	got := underlying.GetMemory()
	if len(got) != 3 {
		t.Fatalf("expected visible transcript only, got %d messages", len(got))
	}
	if got[0].Role != llm.RoleSystem || got[1].Role != llm.RoleUser || got[2].Role != llm.RoleAssistant {
		t.Fatalf("unexpected restored roles: %+v", got)
	}
	if got[2].Content == nil || *got[2].Content != assistant {
		t.Fatalf("unexpected restored assistant content: %+v", got[2])
	}
}
