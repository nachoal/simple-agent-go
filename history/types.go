package history

import (
	"time"
)

// Session represents a conversation session
type Session struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Path      string    `json:"path"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Metadata  Metadata  `json:"metadata"`
	Messages  []Message `json:"messages"`
}

// Metadata contains session metadata
type Metadata struct {
	Title      string   `json:"title"`
	Tags       []string `json:"tags"`
	TokenCount int      `json:"token_count"`
}

// Message represents a conversation message
type Message struct {
	Role       string      `json:"role"`
	Content    *string     `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}

// ToolCall represents a tool invocation
type ToolCall struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall contains function call details
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// MetaIndex contains session indexing information
type MetaIndex struct {
	Version     string              `json:"version"`
	LastSession string              `json:"last_session_id,omitempty"`
	PathIndex   map[string][]string `json:"path_index"`
}

// SessionInfo provides summary information for session listing
type SessionInfo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  int       `json:"messages"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
}