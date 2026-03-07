package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nachoal/simple-agent-go/internal/runlog"
	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

type runlogQueryClient struct{}

func (runlogQueryClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	out := "done"
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: &out,
			},
			FinishReason: "stop",
		}},
		Usage: &llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}, nil
}

func (runlogQueryClient) ChatStream(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, nil
}

func (runlogQueryClient) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }
func (runlogQueryClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}
func (runlogQueryClient) Close() error { return nil }

func TestQuery_WritesStructuredRunLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	logger, err := runlog.New(t.TempDir(), "query-test")
	if err != nil {
		t.Fatalf("runlog.New: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	ctx = runlog.WithContext(ctx, logger)
	ctx = runlog.WithMetadata(ctx, runlog.Metadata{
		RunID:    "run-query-1",
		Mode:     "query",
		Prompt:   "hello",
		Provider: "openai",
		Model:    "fake-model",
	})

	a := New(runlogQueryClient{}, WithTools(nil))
	if _, err := a.Query(ctx, "hello"); err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("logger.Close: %v", err)
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"run_id":"run-query-1"`, `"kind":"llm_request"`, `"kind":"llm_response"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected log to contain %s, got:\n%s", want, text)
		}
	}
}

func TestQueryStream_WritesToolEventsToRunLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := registry.Register(streamContentFallbackToolName, func() tools.Tool {
		return streamContentFallbackTool{}
	}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("failed to register test tool: %v", err)
	}

	logger, err := runlog.New(t.TempDir(), "stream-test")
	if err != nil {
		t.Fatalf("runlog.New: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	ctx = runlog.WithContext(ctx, logger)
	ctx = runlog.WithMetadata(ctx, runlog.Metadata{
		RunID:    "run-stream-1",
		Mode:     "stream",
		Prompt:   "use the tool",
		Provider: "openai",
		Model:    "fake-model",
	})

	a := New(&contentFallbackStreamClient{},
		WithTools([]string{streamContentFallbackToolName}),
		WithMaxIterations(4),
		WithMaxToolCalls(4),
	)

	stream, err := a.QueryStream(ctx, "use the tool")
	if err != nil {
		t.Fatalf("QueryStream returned error: %v", err)
	}
	for range stream {
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("logger.Close: %v", err)
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"run_id":"run-stream-1"`, `"kind":"tool_start"`, `"kind":"tool_result"`, `"kind":"run_complete"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected log to contain %s, got:\n%s", want, text)
		}
	}
}
