package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/llm"
)

// View represents the current view/screen
type View int

const (
	ViewChat View = iota
	ViewTools
	ViewSettings
	ViewHelp
)

// Model represents the main application state
type Model struct {
	// UI components
	textarea    textarea.Model
	chatView    viewport.Model
	toolsView   viewport.Model
	currentView View

	// Agent and conversation
	agent        agent.Agent
	llmClient    llm.Client
	messages     []ChatMessage
	isProcessing bool
	error        error

	// Tool execution
	activeTools []ToolExecution
	toolResults map[string]string

	// Layout
	width       int
	height      int
	showTools   bool
	toolsWidth  int
	initialized bool

	// Configuration
	provider string
	model    string
	theme    Theme

	// Key bindings
	keys KeyMap
}

// ChatMessage represents a message in the chat
type ChatMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
	ToolCalls []ToolCall
}

// ToolCall represents a tool invocation
type ToolCall struct {
	ID       string
	Name     string
	Args     string
	Result   string
	Error    error
	Duration time.Duration
}

// ToolExecution represents an ongoing tool execution
type ToolExecution struct {
	ID        string
	Name      string
	StartTime time.Time
	Status    ToolStatus
}

// ToolStatus represents the status of a tool execution
type ToolStatus int

const (
	ToolStatusPending ToolStatus = iota
	ToolStatusRunning
	ToolStatusComplete
	ToolStatusError
)

// KeyMap defines key bindings
type KeyMap struct {
	Quit        key.Binding
	Send        key.Binding
	Clear       key.Binding
	ToggleHelp  key.Binding
	ToggleTools key.Binding
	NextView    key.Binding
	PrevView    key.Binding
	Copy        key.Binding
	Paste       key.Binding
}

// DefaultKeyMap returns default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "ctrl+d"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Send: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send message"),
		),
		Clear: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear chat"),
		),
		ToggleHelp: key.NewBinding(
			key.WithKeys("?", "f1"),
			key.WithHelp("?", "toggle help"),
		),
		ToggleTools: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "toggle tools panel"),
		),
		NextView: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next view"),
		),
		PrevView: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous view"),
		),
		Copy: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "copy"),
		),
		Paste: key.NewBinding(
			key.WithKeys("ctrl+v"),
			key.WithHelp("ctrl+v", "paste"),
		),
	}
}

// Messages for the update loop
type (
	// AgentResponseMsg is sent when the agent responds
	AgentResponseMsg struct {
		Content   string
		ToolCalls []ToolCall
		Error     error
	}

	// StreamUpdateMsg is sent during streaming responses
	StreamUpdateMsg struct {
		Content string
	}

	// ToolStartMsg is sent when a tool starts execution
	ToolStartMsg struct {
		ID   string
		Name string
		Args string
	}

	// ToolCompleteMsg is sent when a tool completes
	ToolCompleteMsg struct {
		ID     string
		Result string
		Error  error
	}

	// WindowSizeMsg is sent when the window is resized
	WindowSizeMsg struct {
		Width  int
		Height int
	}

	// TickMsg is sent periodically for animations
	TickMsg time.Time
)
