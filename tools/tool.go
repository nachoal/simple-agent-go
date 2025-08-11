package tools

import (
	"context"
	"encoding/json"
)

// Tool defines the interface that all tools must implement
type Tool interface {
	// Name returns the unique name of the tool
	Name() string

	// Description returns a brief description of what the tool does
	Description() string

	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params json.RawMessage) (string, error)

	// Parameters returns a struct that defines the tool's parameters
	// This struct will be used for schema generation
	Parameters() interface{}
}

// ToolError represents a structured error from a tool
type ToolError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *ToolError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// NewToolError creates a new tool error
func NewToolError(code, message string) *ToolError {
	return &ToolError{
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WithDetail adds a detail to the error
func (e *ToolError) WithDetail(key string, value interface{}) *ToolError {
	e.Details[key] = value
	return e
}

// ToolCall represents a request to execute a tool
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Result string `json:"result"`
	Error  error  `json:"error,omitempty"`
}
