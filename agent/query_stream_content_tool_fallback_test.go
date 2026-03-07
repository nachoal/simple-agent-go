package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

const streamContentFallbackToolName = "stream_content_fallback_tool"

type streamContentFallbackParams struct {
	Input string `json:"input"`
}

type streamContentFallbackTool struct{}

func (streamContentFallbackTool) Name() string {
	return streamContentFallbackToolName
}

func (streamContentFallbackTool) Description() string {
	return "Test-only tool for stream JSON-content fallback"
}

func (streamContentFallbackTool) Parameters() interface{} {
	return &streamContentFallbackParams{}
}

func (streamContentFallbackTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p streamContentFallbackParams
	_ = json.Unmarshal(params, &p)
	return "handled:" + p.Input, nil
}

type contentFallbackStreamClient struct {
	mu      sync.Mutex
	calls   int
	payload string
}

func (c *contentFallbackStreamClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}

func (c *contentFallbackStreamClient) ChatStream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()

	ch := make(chan llm.StreamEvent, 4)
	go func() {
		defer close(ch)

		switch call {
		case 1:
			payload := c.payload
			if payload == "" {
				payload = `{"name":"` + streamContentFallbackToolName + `","arguments":{"input":"ping"}}`
			}
			ch <- llm.StreamEvent{
				Choices: []llm.Choice{
					{
						Delta: &llm.Message{
							Content: llm.StringPtr(payload),
						},
					},
				},
			}
		default:
			final := "done"
			ch <- llm.StreamEvent{
				Choices: []llm.Choice{
					{
						Delta: &llm.Message{
							Content: &final,
						},
					},
				},
			}
		}
	}()

	return ch, nil
}

func (c *contentFallbackStreamClient) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (c *contentFallbackStreamClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}

func (c *contentFallbackStreamClient) Close() error {
	return nil
}

type contentFallbackQueryClient struct {
	mu      sync.Mutex
	calls   int
	payload string
}

func (c *contentFallbackQueryClient) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()

	switch call {
	case 1:
		payload := c.payload
		if payload == "" {
			payload = `{"name":"` + streamContentFallbackToolName + `","arguments":{"input":"ping"}}`
		}
		return &llm.ChatResponse{
			Choices: []llm.Choice{
				{
					Message: llm.Message{
						Role:    llm.RoleAssistant,
						Content: llm.StringPtr(payload),
					},
				},
			},
		}, nil
	default:
		final := "done"
		return &llm.ChatResponse{
			Choices: []llm.Choice{
				{
					Message: llm.Message{
						Role:    llm.RoleAssistant,
						Content: &final,
					},
				},
			},
		}, nil
	}
}

func (c *contentFallbackQueryClient) ChatStream(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, nil
}

func (c *contentFallbackQueryClient) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (c *contentFallbackQueryClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}

func (c *contentFallbackQueryClient) Close() error {
	return nil
}

func TestQueryStream_ParsesToolCallFromContentWhenStreaming(t *testing.T) {
	if err := registry.Register(streamContentFallbackToolName, func() tools.Tool {
		return streamContentFallbackTool{}
	}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("failed to register test tool: %v", err)
	}

	client := &contentFallbackStreamClient{}
	a := New(client,
		WithTools([]string{streamContentFallbackToolName}),
		WithMaxIterations(4),
		WithMaxToolCalls(4),
	)

	stream, err := a.QueryStream(context.Background(), "use the tool")
	if err != nil {
		t.Fatalf("QueryStream returned error: %v", err)
	}

	sawToolStart := false
	sawToolResult := false
	sawComplete := false

	for event := range stream {
		switch event.Type {
		case EventTypeToolStart:
			if event.Tool != nil && event.Tool.Name == streamContentFallbackToolName {
				sawToolStart = true
			}
		case EventTypeToolResult:
			if event.Tool != nil && event.Tool.Name == streamContentFallbackToolName && event.Tool.Result == "handled:ping" {
				sawToolResult = true
			}
		case EventTypeComplete:
			sawComplete = true
		}
	}

	if !sawToolStart {
		t.Fatalf("expected stream to emit tool start event")
	}
	if !sawToolResult {
		t.Fatalf("expected stream to emit successful tool result event")
	}
	if !sawComplete {
		t.Fatalf("expected stream to emit completion event")
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls < 2 {
		t.Fatalf("expected at least 2 stream calls (tool call + final response), got %d", client.calls)
	}
}

func TestQueryStream_RecoversMalformedToolCallFromContentWhenStreaming(t *testing.T) {
	if err := registry.Register(streamContentFallbackToolName, func() tools.Tool {
		return streamContentFallbackTool{}
	}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("failed to register test tool: %v", err)
	}

	client := &contentFallbackStreamClient{
		payload: `{"name":"` + streamContentFallbackToolName + `","arguments":{"input":"ping"}</arg_value></tool_call>`,
	}
	a := New(client,
		WithTools([]string{streamContentFallbackToolName}),
		WithMaxIterations(4),
		WithMaxToolCalls(4),
	)

	stream, err := a.QueryStream(context.Background(), "use the tool")
	if err != nil {
		t.Fatalf("QueryStream returned error: %v", err)
	}

	sawToolStart := false
	sawToolResult := false
	sawComplete := false

	for event := range stream {
		switch event.Type {
		case EventTypeToolStart:
			if event.Tool != nil && event.Tool.Name == streamContentFallbackToolName {
				sawToolStart = true
			}
		case EventTypeToolResult:
			if event.Tool != nil && event.Tool.Name == streamContentFallbackToolName && event.Tool.Result == "handled:ping" {
				sawToolResult = true
			}
		case EventTypeComplete:
			sawComplete = true
		}
	}

	if !sawToolStart {
		t.Fatalf("expected malformed content to still emit tool start event")
	}
	if !sawToolResult {
		t.Fatalf("expected malformed content to still emit successful tool result event")
	}
	if !sawComplete {
		t.Fatalf("expected malformed content stream to complete")
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls < 2 {
		t.Fatalf("expected at least 2 stream calls (tool call + final response), got %d", client.calls)
	}
}

func TestQuery_RecoversMalformedToolCallFromContent(t *testing.T) {
	if err := registry.Register(streamContentFallbackToolName, func() tools.Tool {
		return streamContentFallbackTool{}
	}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("failed to register test tool: %v", err)
	}

	client := &contentFallbackQueryClient{
		payload: `{"name":"` + streamContentFallbackToolName + `","arguments":{"input":"ping"}</arg_value></tool_call>`,
	}
	a := New(client,
		WithTools([]string{streamContentFallbackToolName}),
		WithMaxIterations(4),
		WithMaxToolCalls(4),
	)

	resp, err := a.Query(context.Background(), "use the tool")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("expected final response %q, got %q", "done", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Result != "handled:ping" {
		t.Fatalf("expected tool execution result handled:ping, got %#v", resp.ToolCalls)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls < 2 {
		t.Fatalf("expected at least 2 chat calls (tool call + final response), got %d", client.calls)
	}
}

func TestQuery_RecoversMalformedToolCallWithUnclosedStringFromContent(t *testing.T) {
	if err := registry.Register(streamContentFallbackToolName, func() tools.Tool {
		return streamContentFallbackTool{}
	}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("failed to register test tool: %v", err)
	}

	client := &contentFallbackQueryClient{
		payload: `{"name":"` + streamContentFallbackToolName + `","arguments":{"input":"ping</arg_value></tool_call>`,
	}
	a := New(client,
		WithTools([]string{streamContentFallbackToolName}),
		WithMaxIterations(4),
		WithMaxToolCalls(4),
	)

	resp, err := a.Query(context.Background(), "use the tool")
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("expected final response %q, got %q", "done", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Result != "handled:ping" {
		t.Fatalf("expected tool execution result handled:ping, got %#v", resp.ToolCalls)
	}
}
