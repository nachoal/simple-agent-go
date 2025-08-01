package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/config"
	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

// BorderedTUI is a minimal TUI that matches the Python bordered_interface.py
type BorderedTUI struct {
	agent         agent.Agent
	llmClient     llm.Client
	provider      string
	model         string
	textarea      textarea.Model
	messages      []BorderedMessage
	width         int
	height        int
	isThinking    bool
	err           error
	initialized   bool // Track if we've received the first WindowSizeMsg
	
	// Model selection
	selectingModel bool
	modelSelector  tea.Model
	providers      map[string]llm.Client
	configManager  *config.Manager
	
	// Glamour renderer
	renderer *glamour.TermRenderer
	
	// Spinner for thinking state
	spinner spinner.Model
	
	// Tool execution tracking
	activeTools    map[string]*ActiveTool
	completedTools []CompletedTool
	toolErrors     []ToolError
	showingTools   bool
	lastRender     time.Time
	renderPending  bool
	toolEventChan  chan agent.StreamEvent
	toolsUsedInLastQuery map[string]time.Duration
}

// BorderedMessage represents a chat message
type BorderedMessage struct {
	Role    string
	Content string
}

// ActiveTool represents a currently executing tool
type ActiveTool struct {
	ID               string
	Name             string
	Args             map[string]interface{}
	StartTime        time.Time
	Status           ToolStatus
	Output           *CircularBuffer
	Progress         float64
	LastProgressText string
	LastUpdate       time.Time
}


// CompletedTool represents a completed tool execution
type CompletedTool struct {
	ID           string
	Name         string
	CompletedAt  time.Time
	Success      bool
	OutputSample string
}

// ToolError represents a tool error
type ToolError struct {
	ID      string
	Name    string
	Error   error
	Time    time.Time
	Details string
}

// CircularBuffer manages output with a size limit
type CircularBuffer struct {
	lines    []string
	maxLines int
	total    int
}

// NewCircularBuffer creates a new circular buffer
func NewCircularBuffer(maxLines int) *CircularBuffer {
	return &CircularBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

// Add adds a line to the buffer
func (cb *CircularBuffer) Add(line string) {
	cb.total++
	if len(cb.lines) < cb.maxLines {
		cb.lines = append(cb.lines, line)
	} else {
		// Shift and add
		copy(cb.lines, cb.lines[1:])
		cb.lines[len(cb.lines)-1] = line
	}
}

// GetLines returns current lines
func (cb *CircularBuffer) GetLines() []string {
	return cb.lines
}

// HasOverflow returns true if more lines were added than capacity
func (cb *CircularBuffer) HasOverflow() bool {
	return cb.total > cb.maxLines
}

// NewBorderedTUI creates a new bordered TUI
func NewBorderedTUI(llmClient llm.Client, agentInstance agent.Agent, provider, model string) *BorderedTUI {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.ShowLineNumbers = false
	ta.Prompt = "" // Remove the prompt since we'll handle it in the border
	ta.CharLimit = 0
	ta.SetHeight(1) // Start with single line
	ta.Focus()
	
	// Completely transparent styles - unset all backgrounds and borders
	transparentStyle := lipgloss.NewStyle().
		UnsetBackground().
		UnsetBorderBackground().
		UnsetBorderStyle()
	
	// Apply transparent styles to all textarea style states
	ta.FocusedStyle.Base = transparentStyle
	ta.FocusedStyle.Text = transparentStyle
	ta.FocusedStyle.Placeholder = transparentStyle
	ta.FocusedStyle.Prompt = transparentStyle
	ta.FocusedStyle.CursorLine = transparentStyle
	
	ta.BlurredStyle.Base = transparentStyle
	ta.BlurredStyle.Text = transparentStyle
	ta.BlurredStyle.Placeholder = transparentStyle
	ta.BlurredStyle.Prompt = transparentStyle
	ta.BlurredStyle.CursorLine = transparentStyle
	
	// Disable newlines - Enter will send the message
	ta.KeyMap.InsertNewline.SetEnabled(false)
	
	// Set initial width (will be updated by WindowSizeMsg)
	ta.SetWidth(74) // Default width minus borders/padding
	
	// Simple glamour renderer
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(74),
	)
	
	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("75")) // Same color as model
	
	tui := &BorderedTUI{
		agent:       agentInstance,
		llmClient:   llmClient,
		provider:    provider,
		model:       model,
		textarea:    ta,
		messages:    []BorderedMessage{},
		width:       80, // Default terminal width
		initialized: false,
		renderer:    renderer,
		spinner:     s,
		activeTools:    make(map[string]*ActiveTool),
		completedTools: []CompletedTool{},
		toolErrors:     []ToolError{},
		lastRender:     time.Now(),
		toolsUsedInLastQuery: make(map[string]time.Duration),
	}
	
	return tui
}

// NewBorderedTUIWithProviders creates a new bordered TUI with provider and config support
func NewBorderedTUIWithProviders(llmClient llm.Client, agentInstance agent.Agent, provider, model string, providers map[string]llm.Client, configManager *config.Manager) *BorderedTUI {
	tui := NewBorderedTUI(llmClient, agentInstance, provider, model)
	tui.providers = providers
	tui.configManager = configManager
	return tui
}

// NewBorderedTUIWithHistory creates a new bordered TUI with history support
func NewBorderedTUIWithHistory(llmClient llm.Client, historyAgent *agent.HistoryAgent, provider, model string, providers map[string]llm.Client, configManager *config.Manager) *BorderedTUI {
	tui := NewBorderedTUI(llmClient, historyAgent, provider, model)
	tui.providers = providers
	tui.configManager = configManager
	
	// Load messages from history if available
	if historyAgent != nil {
		session := historyAgent.GetSession()
		if session != nil && len(session.Messages) > 0 {
			// Debug output
			if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
				fmt.Fprintf(os.Stderr, "[TUI] Loading %d messages from session %s\n", len(session.Messages), session.ID)
			}
			
			// Convert history messages to TUI messages
			for _, msg := range session.Messages {
				// Skip system messages in the display
				if msg.Role == "system" {
					continue
				}
				
				content := ""
				if msg.Content != nil {
					content = *msg.Content
				}
				
				tui.messages = append(tui.messages, BorderedMessage{
					Role:    msg.Role,
					Content: content,
				})
			}
			
			if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
				fmt.Fprintf(os.Stderr, "[TUI] Loaded %d messages into TUI display\n", len(tui.messages))
			}
		}
	}
	
	return tui
}

func (m BorderedTUI) Init() tea.Cmd {
	// Initialize with a default width if not set
	if m.width == 0 {
		m.width = 80 // Default terminal width
	}
	return textarea.Blink
}



func (m BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Update spinner if we're thinking
	if m.isThinking {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// If we're in model selection mode, delegate to the model selector
	if m.selectingModel && m.modelSelector != nil {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			// Forward window size to model selector
			m.modelSelector, cmd = m.modelSelector.Update(msg)
			return m, cmd
		case tea.KeyMsg:
			// Allow escape or ctrl+c to exit model selection
			if msg.String() == "esc" || msg.String() == "ctrl+c" {
				m.selectingModel = false
				m.modelSelector = nil
				m.textarea.Focus()
				return m, nil
			}
		case modelSelectedMsg:
			// Model was selected, update our state
			m.provider = msg.provider
			m.model = msg.model
			m.selectingModel = false
			m.modelSelector = nil
			
			// Save to config if we have a config manager
			if m.configManager != nil {
				if err := m.configManager.SetDefaults(msg.provider, msg.model); err != nil {
					m.messages = append(m.messages, BorderedMessage{
						Role:    "error",
						Content: fmt.Sprintf("Failed to save config: %v", err),
					})
				}
			}
			
			// Update the agent with the new model
			if newClient, ok := m.providers[msg.provider]; ok {
				m.llmClient = newClient
				m.agent = agent.New(newClient,
					agent.WithMaxIterations(10),
					agent.WithTemperature(0.7),
				)
			}
			
			m.messages = append(m.messages, BorderedMessage{
				Role:    "command",
				Content: fmt.Sprintf("Switched to %s - %s", msg.provider, msg.model),
			})
			m.textarea.Focus()
			return m, nil
		}
		
		// Update the model selector
		m.modelSelector, cmd = m.modelSelector.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Update textarea width to match terminal minus borders and padding
		// The -6 accounts for: border (2) + padding (2) + some margin (2)
		textareaWidth := m.width - 6
		if textareaWidth < 20 {
			textareaWidth = 20 // Minimum width
		}
		m.textarea.SetWidth(textareaWidth)
		
		// Adjust height based on content
		m.adjustTextareaHeight()
		
		// Only clear screen on actual resize, not initial setup
		if m.initialized {
			// This is a resize event, clear the screen
			return m, tea.ClearScreen
		}
		// This is the initial WindowSizeMsg
		m.initialized = true
		return m, nil
		
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlQ, tea.KeyEsc:
			return m, tea.Quit
			
		case tea.KeyCtrlL:
			m.messages = []BorderedMessage{}
			return m, nil
			
		case tea.KeyEnter:
			// Send the message on Enter
			if !m.isThinking {
				value := m.textarea.Value()
				if strings.TrimSpace(value) != "" {
					// Add user message
					m.messages = append(m.messages, BorderedMessage{
						Role:    "user",
						Content: value,
					})
					
					// Clear input and reset height
					m.textarea.Reset()
					m.textarea.SetHeight(1)
					m.textarea.Blur()
					
					
					// Send to agent
					m.isThinking = true
					m.showingTools = false
					
					// Create event channel and store it
					m.toolEventChan = make(chan agent.StreamEvent, 100)
					
					cmds = append(cmds, m.sendMessage(value))
					cmds = append(cmds, m.spinner.Tick)
					cmds = append(cmds, m.listenForToolEvents())
				}
			}
			return m, tea.Batch(cmds...)
		}
		
	case toolEventMsg:
		// Handle tool events
		if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[TUI] Received tool event: %s\n", msg.event.Type)
			if msg.event.Tool != nil {
				fmt.Fprintf(os.Stderr, "[TUI] Tool: %s (ID: %s)\n", msg.event.Tool.Name, msg.event.Tool.ID)
			}
		}
		
		switch msg.event.Type {
		case agent.EventTypeToolStart:
			if msg.event.Tool != nil {
				// Keep showing "Thinking..." when tools start - show continuous indicator
				m.isThinking = true
				m.showingTools = true
				
				// Add to active tools
				m.activeTools[msg.event.Tool.ID] = &ActiveTool{
					ID:        msg.event.Tool.ID,
					Name:      msg.event.Tool.Name,
					Args:      msg.event.Tool.Args,
					StartTime: time.Now(),
					Status:    ToolStatusRunning,
					Output:    NewCircularBuffer(20),
				}
				
				// Track tool usage
				m.toolsUsedInLastQuery[msg.event.Tool.Name] = 0
				
				// Add tool start message to chat history
				argStr := m.formatArguments(msg.event.Tool.Args)
				toolStartMsg := fmt.Sprintf("🔧 Calling tool: %s %s", msg.event.Tool.Name, argStr)
				m.messages = append(m.messages, BorderedMessage{
					Role:    "tool_info",
					Content: toolStartMsg,
				})
			}
			
		case agent.EventTypeToolProgress:
			if msg.event.Tool != nil && m.activeTools[msg.event.Tool.ID] != nil {
				tool := m.activeTools[msg.event.Tool.ID]
				tool.Progress = msg.event.Tool.Progress
				tool.LastProgressText = msg.event.Tool.Message
				tool.LastUpdate = time.Now()
			}
			
		case agent.EventTypeToolResult:
			if msg.event.Tool != nil {
				// Move from active to completed
				if activeTool := m.activeTools[msg.event.Tool.ID]; activeTool != nil {
					delete(m.activeTools, msg.event.Tool.ID)
					
					// Add to completed
					completedTool := CompletedTool{
						ID:           msg.event.Tool.ID,
						Name:         activeTool.Name,
						CompletedAt:  time.Now(),
						Success:      msg.event.Tool.Error == nil,
						OutputSample: msg.event.Tool.Result,
					}
					m.completedTools = append(m.completedTools, completedTool)
					
					// Update duration in tracking
					duration := time.Since(activeTool.StartTime)
					m.toolsUsedInLastQuery[activeTool.Name] = duration
					
					// Add tool completion message to chat history
					if msg.event.Tool.Error != nil {
						// Add error message
						m.toolErrors = append(m.toolErrors, ToolError{
							ID:    msg.event.Tool.ID,
							Name:  activeTool.Name,
							Error: msg.event.Tool.Error,
							Time:  time.Now(),
						})
						
						errorMsg := fmt.Sprintf("❌ Tool %s failed: %v", activeTool.Name, msg.event.Tool.Error)
						m.messages = append(m.messages, BorderedMessage{
							Role:    "tool_info",
							Content: errorMsg,
						})
					} else {
						// Add success message with duration
						successMsg := fmt.Sprintf("✅ Tool %s completed in %v", activeTool.Name, duration.Round(time.Millisecond))
						m.messages = append(m.messages, BorderedMessage{
							Role:    "tool_info",
							Content: successMsg,
						})
					}
				}
			}
		}
		
		// Continue listening for more events
		return m, m.listenForToolEvents()
		
	case borderedResponseMsg:
		m.isThinking = false
		m.showingTools = false
		
		// Reset for next query
		m.toolsUsedInLastQuery = make(map[string]time.Duration)
		m.activeTools = make(map[string]*ActiveTool)
		m.completedTools = []CompletedTool{}
		
		// Handle special command cases
		if msg.isQuit {
			return m, tea.Quit
		}
		
		if msg.isClear {
			m.messages = []BorderedMessage{}
			m.textarea.Focus()
			return m, nil
		}
		
		if msg.isModelSelect {
			// Trigger model selection
			m.selectingModel = true
			m.textarea.Blur()
			
			// Create the model selector with a callback
			onSelect := func(provider, model string) tea.Cmd {
				return func() tea.Msg {
					return modelSelectedMsg{provider: provider, model: model}
				}
			}
			
			m.modelSelector = NewModelSelector(m.providers, onSelect)
			// Send the current window size to the model selector
			if m.width > 0 && m.height > 0 {
				m.modelSelector, _ = m.modelSelector.Update(tea.WindowSizeMsg{
					Width:  m.width,
					Height: m.height,
				})
			}
			return m, m.modelSelector.Init()
		}
		
		// Handle normal messages
		if msg.err != nil {
			m.messages = append(m.messages, BorderedMessage{
				Role:    "error",
				Content: fmt.Sprintf("Error: %v", msg.err),
			})
		} else if msg.content != "" {
			role := "assistant"
			if msg.isCommand {
				role = "command"
			}
			m.messages = append(m.messages, BorderedMessage{
				Role:    role,
				Content: msg.content,
			})
		}
		m.textarea.Focus()
		return m, nil
	}

	// Update textarea
	oldValue := m.textarea.Value()
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	
	// Check if content changed to adjust height
	if oldValue != m.textarea.Value() {
		m.adjustTextareaHeight()
	}

	return m, tea.Batch(cmds...)
}

func (m BorderedTUI) View() string {
	// If we're showing the model selector, display it instead
	if m.selectingModel && m.modelSelector != nil {
		return m.modelSelector.View()
	}
	
	// Ensure we have a valid width
	width := m.width
	if width == 0 {
		width = 80 // Default fallback
	}
	
	// Colors matching Python version
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	modelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))  // #5B9BD5
	toolsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("80"))  // #4ECDC4
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))   // #6B7280
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("15"))
	
	var b strings.Builder
	
	// Header - matching Python version exactly
	verboseIndicator := ""
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		verboseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // Red
		verboseIndicator = " | " + verboseStyle.Render("[VERBOSE]")
	}
	
	header1 := fmt.Sprintf("%s | Model: %s | Provider: %s%s",
		headerStyle.Render("Simple Agent Go"),
		modelStyle.Render(m.model),
		modelStyle.Render(m.provider),
		verboseIndicator)
	
	toolCount := len(registry.List())
	header2 := fmt.Sprintf("%s | %s",
		toolsStyle.Render(fmt.Sprintf("Loaded %d tools", toolCount)),
		cmdStyle.Render("Commands: /help, /tools, /model, /system, /verbose, /clear, /exit"))
	
	b.WriteString(header1 + "\n")
	b.WriteString(header2 + "\n\n")
	
	// Messages
	// Create a style for wrapping text based on terminal width
	messageStyle := lipgloss.NewStyle().Width(width - 4) // Leave some margin
	
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			// Render with emoji prefix and wrapped text
			content := messageStyle.Render(fmt.Sprintf("👤 You: %s", msg.Content))
			b.WriteString(content + "\n")
		case "assistant":
			// Use glamour if available, otherwise fallback
			if m.renderer != nil {
				rendered, err := m.renderer.Render(msg.Content)
				if err == nil {
					b.WriteString("🤖 Assistant:\n")
					b.WriteString(strings.TrimRight(rendered, "\n") + "\n")
				} else {
					// Fallback
					content := messageStyle.Render(fmt.Sprintf("🤖 Assistant: %s", msg.Content))
					b.WriteString(content + "\n")
				}
			} else {
				// No renderer
				content := messageStyle.Render(fmt.Sprintf("🤖 Assistant: %s", msg.Content))
				b.WriteString(content + "\n")
			}
		case "command":
			// Command output - no prefix, just the content
			content := messageStyle.Render(msg.Content)
			b.WriteString(content + "\n")
		case "error":
			// Render with emoji prefix and wrapped text
			content := messageStyle.Render(fmt.Sprintf("❌ %s", msg.Content))
			b.WriteString(content + "\n")
		case "tool_info":
			// Tool usage information - style differently
			toolInfoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
			content := toolInfoStyle.Render(msg.Content)
			b.WriteString(content + "\n")
		}
		b.WriteString("\n")
	}
	
	// Show thinking indicator with spinner
	if m.isThinking {
		b.WriteString(fmt.Sprintf("%s Thinking...\n\n", m.spinner.View()))
	}
	
	// Input area with border and prompt
	inputContent := m.textarea.View()
	// Add the prompt prefix
	promptedInput := "> " + inputContent
	
	// Style the input box with border
	styledInput := borderStyle.
		Width(width - 2).
		PaddingLeft(1).
		PaddingRight(1).
		Render(promptedInput)
	b.WriteString(styledInput)
	
	return b.String()
}

func (m *BorderedTUI) sendMessage(input string) tea.Cmd {
	return func() tea.Msg {
		// Handle commands
		if strings.HasPrefix(input, "/") {
			return m.handleCommand(input)
		}
		
		// Query the agent with event channel in context
		ctx := context.Background()
		if m.toolEventChan != nil {
			ctx = context.WithValue(ctx, "toolEventChan", m.toolEventChan)
		}
		
		response, err := m.agent.Query(ctx, input)
		
		// Close the event channel after query completes
		if m.toolEventChan != nil {
			close(m.toolEventChan)
		}
		
		if err != nil {
			return borderedResponseMsg{err: err}
		}
		
		return borderedResponseMsg{content: response.Content}
	}
}

func (m *BorderedTUI) handleCommand(cmd string) borderedResponseMsg {
	switch strings.ToLower(strings.TrimSpace(cmd)) {
	case "/exit", "/quit":
		// Return a special message type that will trigger quit
		return borderedResponseMsg{content: "", isQuit: true}
	case "/clear":
		// Return a special message type that will trigger clear
		return borderedResponseMsg{content: "", isClear: true}
	case "/help":
		help := `Commands:
  /help    - Show this help
  /tools   - List available tools
  /model   - Change model interactively
  /system  - Show system prompt
  /verbose - Toggle verbose/debug mode
  /clear   - Clear chat history
  /exit    - Exit application

Keyboard shortcuts:
  Ctrl+C - Quit
  Ctrl+L - Clear chat
  Enter  - Send message`
		return borderedResponseMsg{content: help, isCommand: true}
	case "/tools":
		var toolsBuilder strings.Builder
		toolsBuilder.WriteString("Available tools:\n")
		
		// Get all tools from registry
		toolNames := registry.List()
		for _, name := range toolNames {
			tool, err := registry.Get(name)
			if err != nil {
				continue
			}
			// Format: tool_name - description
			toolsBuilder.WriteString(fmt.Sprintf("  %-15s - %s\n", name, tool.Description()))
		}
		
		return borderedResponseMsg{content: strings.TrimRight(toolsBuilder.String(), "\n"), isCommand: true}
	case "/model":
		// Check if providers are available
		if m.providers == nil || len(m.providers) == 0 {
			return borderedResponseMsg{content: "Model selection not available (no providers configured)", isCommand: true}
		}
		
		// Return a special message that will trigger model selection
		return borderedResponseMsg{content: "", isModelSelect: true}
	case "/system":
		// Show the current system prompt with tools
		messages := m.agent.GetMemory()
		if len(messages) > 0 && messages[0].Role == "system" {
			return borderedResponseMsg{
				content: fmt.Sprintf("**Current System Prompt (including tools):**\n\n%s", messages[0].Content),
				isCommand: true,
			}
		}
		// Fallback to default if no system message found
		systemPrompt := agent.DefaultConfig().SystemPrompt
		return borderedResponseMsg{
			content: fmt.Sprintf("**Default System Prompt:**\n\n%s", systemPrompt),
			isCommand: true,
		}
	case "/verbose":
		// Toggle verbose mode
		currentDebug := os.Getenv("SIMPLE_AGENT_DEBUG")
		if currentDebug == "true" {
			os.Unsetenv("SIMPLE_AGENT_DEBUG")
			return borderedResponseMsg{content: "Verbose mode: OFF", isCommand: true}
		} else {
			os.Setenv("SIMPLE_AGENT_DEBUG", "true")
			return borderedResponseMsg{content: "Verbose mode: ON\nDebug output will be shown in the terminal", isCommand: true}
		}
	default:
		return borderedResponseMsg{content: fmt.Sprintf("Unknown command: %s", cmd), isCommand: true}
	}
}

type borderedResponseMsg struct {
	content       string
	err           error
	isQuit        bool
	isClear       bool
	isCommand     bool // Flag to indicate this is a command response
	isModelSelect bool // Flag to trigger model selection
}

// modelSelectedMsg is sent when a model is selected
type modelSelectedMsg struct {
	provider string
	model    string
}

// toolEventMsg carries tool execution events
type toolEventMsg struct {
	event agent.StreamEvent
}


// adjustTextareaHeight dynamically adjusts the textarea height based on content
func (m *BorderedTUI) adjustTextareaHeight() {
	content := m.textarea.Value()
	if content == "" {
		m.textarea.SetHeight(1)
		return
	}
	
	// Count lines needed considering word wrapping
	lines := 1
	currentLineLength := 0
	textareaWidth := m.width - 8 // Account for borders, padding, and prompt
	if textareaWidth < 20 {
		textareaWidth = 20
	}
	
	for _, char := range content {
		if char == '\n' {
			lines++
			currentLineLength = 0
		} else {
			currentLineLength++
			if currentLineLength >= textareaWidth {
				lines++
				currentLineLength = 0
			}
		}
	}
	
	// Set height with a reasonable maximum
	maxHeight := 10
	if lines > maxHeight {
		lines = maxHeight
	}
	
	m.textarea.SetHeight(lines)
}

// formatArguments formats tool arguments for display
func (m *BorderedTUI) formatArguments(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	
	return fmt.Sprintf("(%s)", strings.Join(parts, ", "))
}

// listenForToolEvents creates a command that listens for tool events
func (m *BorderedTUI) listenForToolEvents() tea.Cmd {
	return func() tea.Msg {
		if m.toolEventChan == nil {
			return nil
		}
		
		event, ok := <-m.toolEventChan
		if !ok {
			// Channel closed
			return nil
		}
		
		return toolEventMsg{event: event}
	}
}