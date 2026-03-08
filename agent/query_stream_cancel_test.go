package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

type cancelStreamClient struct{}

func (cancelStreamClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}

func (cancelStreamClient) ChatStream(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	events := make(chan llm.StreamEvent)
	go func() {
		<-ctx.Done()
		close(events)
	}()
	return events, nil
}

func (cancelStreamClient) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (cancelStreamClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}

func (cancelStreamClient) Close() error {
	return nil
}

const preserveCommittedCancelToolName = "preserve_committed_cancel_tool"

type preserveCommittedCancelToolParams struct {
	Input string `json:"input"`
}

type preserveCommittedCancelTool struct{}

func (preserveCommittedCancelTool) Name() string { return preserveCommittedCancelToolName }

func (preserveCommittedCancelTool) Description() string {
	return "Test-only tool that blocks until the context is cancelled"
}

func (preserveCommittedCancelTool) Parameters() interface{} {
	return &preserveCommittedCancelToolParams{}
}

func (preserveCommittedCancelTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type committedCancelStreamClient struct{}

func (committedCancelStreamClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}

func (committedCancelStreamClient) ChatStream(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	go func() {
		defer close(ch)
		ch <- llm.StreamEvent{
			Choices: []llm.Choice{{
				Delta: &llm.Message{
					ToolCalls: []llm.ToolCall{{
						ID:   "tc-preserve",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      preserveCommittedCancelToolName,
							Arguments: json.RawMessage(`{"input":"keep"}`),
						},
					}},
				},
			}},
		}
	}()
	return ch, nil
}

func (committedCancelStreamClient) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (committedCancelStreamClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}

func (committedCancelStreamClient) Close() error {
	return nil
}

func TestQueryStream_RollsBackMemoryOnCancelBeforeAssistantCommit(t *testing.T) {
	a := New(cancelStreamClient{}).(*agent)
	initialMemory := a.GetMemory()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := a.QueryStream(ctx, "what time is it?")
	if err != nil {
		t.Fatalf("unexpected QueryStream error: %v", err)
	}

	// Let the streaming loop start, then cancel like Esc in the TUI.
	_, _ = <-stream
	cancel()

	for range stream {
	}

	if got := a.GetMemory(); !reflect.DeepEqual(got, initialMemory) {
		t.Fatalf("expected memory rollback after cancellation")
	}
}

func TestQueryStream_PreservesCommittedMemoryOnCancel(t *testing.T) {
	if err := registry.Register(preserveCommittedCancelToolName, func() tools.Tool {
		return preserveCommittedCancelTool{}
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	a := New(committedCancelStreamClient{}, WithTools([]string{preserveCommittedCancelToolName})).(*agent)
	initialMemory := a.GetMemory()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := a.QueryStream(ctx, "use the tool and wait")
	if err != nil {
		t.Fatalf("unexpected QueryStream error: %v", err)
	}

	for event := range stream {
		if event.Type == EventTypeToolStart {
			cancel()
		}
	}

	got := a.GetMemory()
	if reflect.DeepEqual(got, initialMemory) {
		t.Fatalf("expected committed memory to survive cancellation")
	}
	if len(got) <= len(initialMemory) {
		t.Fatalf("expected additional messages after cancellation, got %d vs %d", len(got), len(initialMemory))
	}
	if got[len(got)-2].Role != llm.RoleAssistant {
		t.Fatalf("expected assistant tool-call message to be preserved, got role %q", got[len(got)-2].Role)
	}
	if got[len(got)-1].Role != llm.RoleTool {
		t.Fatalf("expected tool result message to be preserved, got role %q", got[len(got)-1].Role)
	}
}
