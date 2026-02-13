package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/nachoal/simple-agent-go/llm"
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

func TestQueryStream_RollsBackMemoryOnCancel(t *testing.T) {
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
