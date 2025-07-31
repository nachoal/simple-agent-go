package llm

import (
	"encoding/json"
	"time"
)

// Role represents the role of a message
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a chat message
type Message struct {
	Role       Role            `json:"role"`
	Content    *string         `json:"content,omitempty"`      // Pointer to allow nil/omission
	Name       string          `json:"name,omitempty"`         // For tool messages
	ToolCallID string          `json:"tool_call_id,omitempty"` // For tool responses
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`   // For assistant messages
}

// ToolCall represents a function/tool call request
type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "function"
	Function FunctionCall    `json:"function"`
}

// FunctionCall contains the function name and arguments
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// MarshalJSON customizes JSON serialization for FunctionCall
func (fc FunctionCall) MarshalJSON() ([]byte, error) {
	type Alias FunctionCall
	return json.Marshal(&struct {
		Arguments string `json:"arguments"`
		*Alias
	}{
		Arguments: string(fc.Arguments),
		Alias:     (*Alias)(&fc),
	})
}

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Model            string                   `json:"model"`
	Messages         []Message                `json:"messages"`
	Temperature      float32                  `json:"temperature,omitempty"`
	MaxTokens        int                      `json:"max_tokens,omitempty"`
	TopP             float32                  `json:"top_p,omitempty"`
	Stream           bool                     `json:"stream,omitempty"`
	Tools            []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice       interface{}              `json:"tool_choice,omitempty"` // "auto", "none", or specific tool
	ResponseFormat   *ResponseFormat          `json:"response_format,omitempty"`
	FrequencyPenalty float32                  `json:"frequency_penalty,omitempty"`
	PresencePenalty  float32                  `json:"presence_penalty,omitempty"`
	Stop             []string                 `json:"stop,omitempty"`
}

// ResponseFormat specifies the format of the response
type ResponseFormat struct {
	Type string `json:"type"` // "text" or "json_object"
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []Choice       `json:"choices"`
	Usage             *Usage         `json:"usage,omitempty"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
	Error             *ErrorResponse `json:"error,omitempty"`
}

// Choice represents a single response choice
type Choice struct {
	Index        int          `json:"index"`
	Message      Message      `json:"message"`
	FinishReason string       `json:"finish_reason"` // "stop", "length", "tool_calls", etc.
	Delta        *Message     `json:"delta,omitempty"` // For streaming
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// StreamEvent represents a server-sent event for streaming
type StreamEvent struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []Choice      `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

// ClientOptions contains options for creating an LLM client
type ClientOptions struct {
	APIKey         string
	BaseURL        string
	Timeout        time.Duration
	MaxRetries     int
	DefaultModel   string
	Organization   string
	Headers        map[string]string
}

// ClientOption is a functional option for configuring clients
type ClientOption func(*ClientOptions)

// WithAPIKey sets the API key
func WithAPIKey(key string) ClientOption {
	return func(o *ClientOptions) {
		o.APIKey = key
	}
}

// WithBaseURL sets the base URL
func WithBaseURL(url string) ClientOption {
	return func(o *ClientOptions) {
		o.BaseURL = url
	}
}

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.Timeout = timeout
	}
}

// WithModel sets the default model
func WithModel(model string) ClientOption {
	return func(o *ClientOptions) {
		o.DefaultModel = model
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(retries int) ClientOption {
	return func(o *ClientOptions) {
		o.MaxRetries = retries
	}
}

// WithOrganization sets the organization ID
func WithOrganization(org string) ClientOption {
	return func(o *ClientOptions) {
		o.Organization = org
	}
}

// WithHeaders sets additional headers
func WithHeaders(headers map[string]string) ClientOption {
	return func(o *ClientOptions) {
		if o.Headers == nil {
			o.Headers = make(map[string]string)
		}
		for k, v := range headers {
			o.Headers[k] = v
		}
	}
}

// StringPtr is a helper function to get a pointer to a string
func StringPtr(s string) *string {
	return &s
}

// GetStringValue safely gets string value from pointer
func GetStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}