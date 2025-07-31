package llm

import (
	"context"
	"io"
)

// Client defines the interface for LLM providers
type Client interface {
	// Chat sends a chat request and returns the response
	Chat(ctx context.Context, request *ChatRequest) (*ChatResponse, error)

	// ChatStream sends a chat request and returns a stream of responses
	ChatStream(ctx context.Context, request *ChatRequest) (<-chan StreamEvent, error)

	// ListModels returns available models
	ListModels(ctx context.Context) ([]Model, error)

	// GetModel returns details about a specific model
	GetModel(ctx context.Context, modelID string) (*Model, error)

	// Close cleans up any resources
	Close() error
}

// Model represents an available model
type Model struct {
	ID          string   `json:"id"`
	Object      string   `json:"object"`
	Created     int64    `json:"created"`
	OwnedBy     string   `json:"owned_by"`
	Permission  []string `json:"permission,omitempty"`
	Root        string   `json:"root,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Description string   `json:"description,omitempty"`
}

// StreamReader provides a reader interface for streaming responses
type StreamReader interface {
	io.ReadCloser
	// Next reads the next event from the stream
	Next() (*StreamEvent, error)
}