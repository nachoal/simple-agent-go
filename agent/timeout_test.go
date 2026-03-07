package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
)

type timeoutQueryClient struct{}

func (timeoutQueryClient) Chat(ctx context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (timeoutQueryClient) ChatStream(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (timeoutQueryClient) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }
func (timeoutQueryClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}
func (timeoutQueryClient) Close() error { return nil }

func TestQuery_UsesConfiguredRequestTimeout(t *testing.T) {
	a := New(timeoutQueryClient{}, WithTools(nil), WithTimeout(25*time.Millisecond))

	start := time.Now()
	_, err := a.Query(context.Background(), "slow request")
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Fatalf("query timeout took too long: %v", time.Since(start))
	}
}
