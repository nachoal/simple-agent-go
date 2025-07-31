package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/llm"
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
		
		// Handle normal messages
		if msg.err != nil {
			m.messages = append(m.messages, BorderedMessage{
				Role:    "error",
				Content: fmt.Sprintf("Error: %v", msg.err),
			})
		} else if msg.content != "" {
			m.messages = append(m.messages, BorderedMessage{
				Role:    "assistant",
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
	
	toolCount := 8 // We have 8 tools
	header2 := fmt.Sprintf("%s | %s",
		toolsStyle.Render(fmt.Sprintf("Loaded %d tools", toolCount)),
		cmdStyle.Render("Commands: /help, /tools, /clear, /exit"))
	
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
  /clear - Clear chat history
  /exit  - Exit application

Keyboard shortcuts:
  Ctrl+C - Quit
  Ctrl+L - Clear chat
  Enter  - Send message`
		return borderedResponseMsg{content: help}
	case "/tools":
		tools := `Available tools:
  🧮 calculate       - Evaluate mathematical expressions
  📁 directory_list  - List directory contents
  📝 file_edit       - Edit files by replacing strings
  📄 file_read       - Read file contents
  💾 file_write      - Write content to files
  🔍 google_search   - Search Google
  🖥️  shell          - Execute shell commands
  📚 wikipedia       - Search Wikipedia`
		return borderedResponseMsg{content: tools}
	default:
		return borderedResponseMsg{content: fmt.Sprintf("Unknown command: %s", cmd)}
	}
}

type borderedResponseMsg struct {
	content string
	err     error
	isQuit  bool
	isClear bool
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