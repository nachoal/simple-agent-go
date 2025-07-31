package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
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
}

// BorderedMessage represents a chat message
type BorderedMessage struct {
	Role    string
	Content string
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
	
	return &BorderedTUI{
		agent:     agentInstance,
		llmClient: llmClient,
		provider:  provider,
		model:       model,
		textarea:    ta,
		messages:    []BorderedMessage{},
		width:       80, // Default terminal width
		initialized: false,
	}
}

// NewBorderedTUIWithProviders creates a new bordered TUI with provider and config support
func NewBorderedTUIWithProviders(llmClient llm.Client, agentInstance agent.Agent, provider, model string, providers map[string]llm.Client, configManager *config.Manager) *BorderedTUI {
	tui := NewBorderedTUI(llmClient, agentInstance, provider, model)
	tui.providers = providers
	tui.configManager = configManager
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
					cmds = append(cmds, m.sendMessage(value))
				}
			}
			return m, tea.Batch(cmds...)
		}
		
	case borderedResponseMsg:
		m.isThinking = false
		
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
	header1 := fmt.Sprintf("%s | Model: %s | Provider: %s",
		headerStyle.Render("Simple Agent Go"),
		modelStyle.Render(m.model),
		modelStyle.Render(m.provider))
	
	toolCount := len(registry.List())
	header2 := fmt.Sprintf("%s | %s",
		toolsStyle.Render(fmt.Sprintf("Loaded %d tools", toolCount)),
		cmdStyle.Render("Commands: /help, /tools, /model, /clear, /exit"))
	
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
			// Render with emoji prefix and wrapped text
			content := messageStyle.Render(fmt.Sprintf("🤖 Assistant: %s", msg.Content))
			b.WriteString(content + "\n")
		case "command":
			// Command output - no prefix, just the content
			content := messageStyle.Render(msg.Content)
			b.WriteString(content + "\n")
		case "error":
			// Render with emoji prefix and wrapped text
			content := messageStyle.Render(fmt.Sprintf("❌ %s", msg.Content))
			b.WriteString(content + "\n")
		}
		b.WriteString("\n")
	}
	
	// Show thinking indicator
	if m.isThinking {
		b.WriteString("🤔 Thinking...\n\n")
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
		
		// Query the agent
		ctx := context.Background()
		response, err := m.agent.Query(ctx, input)
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
  /help  - Show this help
  /tools - List available tools
  /model - Change model interactively
  /clear - Clear chat history
  /exit  - Exit application

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