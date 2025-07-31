package agent

import (
	"context"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools"
)

// Config contains agent configuration
type Config struct {
	SystemPrompt    string
	MaxIterations   int
	Temperature     float32
	MaxTokens       int
	Tools           []string
	Verbose         bool
	Timeout         time.Duration
	MemorySize      int
	StreamResponses bool
}

// DefaultConfig returns a default agent configuration
func DefaultConfig() Config {
	return Config{
		SystemPrompt:    defaultSystemPrompt,
		MaxIterations:   10,
		Temperature:     0.7,
		MaxTokens:       2048,
		Verbose:         false,
		Timeout:         5 * time.Minute,
		MemorySize:      100,
		StreamResponses: true,
	}
}

// Memory represents the agent's conversation memory
type Memory struct {
	Messages  []llm.Message
	MaxSize   int
	TokenCount int
}

// Response represents an agent response
type Response struct {
	Content      string
	ToolCalls    []ToolResult
	Usage        *llm.Usage
	FinishReason string
	Error        error
}

// ToolResult is an alias for tools.ToolResult
type ToolResult = tools.ToolResult

// StreamEvent represents an event in the response stream
type StreamEvent struct {
	Type    EventType
	Content string
	Tool    *ToolEvent
	Error   error
}

// EventType represents the type of stream event
type EventType string

const (
	EventTypeMessage    EventType = "message"
	EventTypeToolStart  EventType = "tool_start"
	EventTypeToolResult EventType = "tool_result"
	EventTypeError      EventType = "error"
	EventTypeComplete   EventType = "complete"
)

// ToolEvent contains information about a tool execution
type ToolEvent struct {
	Name   string
	Args   string
	Result string
	Error  error
}

// Agent interface defines the agent contract
type Agent interface {
	// Query sends a query and returns the response
	Query(ctx context.Context, query string) (*Response, error)

	// QueryStream sends a query and streams the response
	QueryStream(ctx context.Context, query string) (<-chan StreamEvent, error)

	// Clear clears the conversation memory
	Clear()

	// GetMemory returns the current conversation memory
	GetMemory() []llm.Message

	// SetSystemPrompt updates the system prompt
	SetSystemPrompt(prompt string)
}

const defaultSystemPrompt = `You are a helpful AI assistant with access to various tools to help answer questions and perform tasks.

When you need to use a tool, you will be provided with function signatures. Use them to gather information or perform actions as needed.

Guidelines:
1. Be helpful, accurate, and concise in your responses
2. Use tools when they would provide more accurate or up-to-date information
3. Explain your reasoning when appropriate
4. If you're unsure about something, say so
5. Always strive to provide the most helpful response possible

Available tools will be provided based on the context.`