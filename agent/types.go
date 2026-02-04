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
	MaxToolCalls    int
	Temperature     float32
	MaxTokens       int
	TopP            float32
	ExtraBody       map[string]interface{}
	Tools           []string
	Verbose         bool
	Timeout         time.Duration
	MemorySize      int
	StreamResponses bool
	progressHandler func(ProgressEvent) // temporary storage for handler
	// Feature flags
	EnableLMStudioParser bool // Parse LM Studio channel-markup tool calls when true
}

// DefaultConfig returns a default agent configuration
func DefaultConfig() Config {
	return Config{
		SystemPrompt:         defaultSystemPrompt,
		MaxIterations:        1000,
		MaxToolCalls:         1000,
		Temperature:          0.7,
		MaxTokens:            2048,
		TopP:                 0,
		ExtraBody:            nil,
		Verbose:              false,
		Timeout:              5 * time.Minute,
		MemorySize:           100,
		StreamResponses:      true,
		EnableLMStudioParser: false,
	}
}

// RequestParams controls per-request model parameters.
type RequestParams struct {
	Temperature float32
	TopP        float32
	ExtraBody   map[string]interface{}
}

// Memory represents the agent's conversation memory
type Memory struct {
	Messages   []llm.Message
	MaxSize    int
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
	EventTypeMessage      EventType = "message"
	EventTypeToolStart    EventType = "tool_start"
	EventTypeToolProgress EventType = "tool_progress"
	EventTypeToolResult   EventType = "tool_result"
	EventTypeToolTimeout  EventType = "tool_timeout"
	EventTypeToolCancel   EventType = "tool_cancel"
	EventTypeThinking     EventType = "thinking" // LLM is reasoning
	EventTypeError        EventType = "error"
	EventTypeComplete     EventType = "complete"
)

// ToolEvent contains information about a tool execution
type ToolEvent struct {
	ID       string                 // Unique tool execution ID
	Name     string                 // Tool name
	Args     map[string]interface{} // Parsed arguments
	ArgsRaw  string                 // Raw JSON string
	Result   string                 // Execution result
	Error    error                  // Execution error
	Progress float64                // Progress percentage (0-1)
	Message  string                 // Progress message
}

// ProgressEvent represents agent progress events
type ProgressEvent struct {
	Type      ProgressEventType
	Iteration int
	Max       int
	ToolCount int
	ToolName  string
	Message   string
}

// ProgressEventType represents types of progress events
type ProgressEventType string

const (
	ProgressEventIteration      ProgressEventType = "iteration"
	ProgressEventToolCallsStart ProgressEventType = "tool_calls_start"
	ProgressEventToolCall       ProgressEventType = "tool_call"
	ProgressEventNoTools        ProgressEventType = "no_tools"
)

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

	// SetMemory sets the conversation memory
	SetMemory(messages []llm.Message)

	// SetRequestParams updates per-request model parameters
	SetRequestParams(params RequestParams)

	// GetRequestParams returns the current per-request model parameters
	GetRequestParams() RequestParams
}

const defaultSystemPrompt = `You are an AI assistant that can leverage external tools to answer the user.
You have access to a set of tools defined separately in the request. When useful, call them.
When you don't call a tool use markdown to format your response.

Guidelines:
1. If the answer can be given directly, do so.
2. If you need to look up information, call the relevant tool. Do NOT fabricate tool calls.
3. A tool call response will be provided with role "tool". You can combine multiple tool calls if helpful.
4. After you have enough information, respond to the user with a clear final answer.

When calling a tool, you have two options:
1. Use the native function calling format if your model supports it (preferred)
2. Respond with **ONLY** a JSON payload following this format:
   {"name": "tool_name", "arguments": {"param1": "value1", "param2": "value2"}}
   Do **not** add any other text when using JSON format—just output the JSON.`
