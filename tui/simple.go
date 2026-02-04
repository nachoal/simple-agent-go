package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/llm"
)

// SimpleModel is a minimal TUI model
type SimpleModel struct {
	// Core components
	agent     agent.Agent
	llmClient llm.Client
	provider  string
	model     string

	// UI components
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model

	// State
	messages     []Message
	isProcessing bool
	width        int
	height       int
	ready        bool

	// Tool tracking
	toolCount   int
	activeTools []string
}

// Message represents a chat message
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// NewSimple creates a minimal TUI
func NewSimple(llmClient llm.Client, agentInstance agent.Agent, provider, model string) *SimpleModel {
	// Create textarea
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false

	// Remove special key bindings to keep it simple
	ta.KeyMap.InsertNewline.SetEnabled(false) // Enter sends message

	// Create spinner
	s := spinner.New(spinner.WithSpinner(spinner.Line))
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))

	return &SimpleModel{
		agent:     agentInstance,
		llmClient: llmClient,
		provider:  provider,
		model:     model,
		textarea:  ta,
		spinner:   s,
		messages:  []Message{},
		toolCount: 6, // Count of loaded tools
	}
}

func (m SimpleModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		textarea.Blink,
	)
}

func (m SimpleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Initialize viewport
			m.viewport = viewport.New(msg.Width, msg.Height-7) // Leave room for header and input
			m.viewport.Style = lipgloss.NewStyle()
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 7
		}

		// Update textarea width
		m.textarea.SetWidth(msg.Width - 4) // Small margin
		m.textarea.SetHeight(3)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlD:
			return m, tea.Quit

		case tea.KeyCtrlL:
			// Clear conversation
			m.messages = []Message{}
			m.viewport.SetContent("")
			return m, nil

		case tea.KeyEnter:
			if !m.isProcessing {
				value := m.textarea.Value()
				if strings.TrimSpace(value) != "" {
					m.handleInput(value)
					m.textarea.Reset()
					cmds = append(cmds, m.sendMessage(value))
				}
			}
			return m, tea.Batch(cmds...)

		case tea.KeyCtrlC:
			if m.textarea.Value() != "" {
				m.textarea.Reset()
			} else {
				return m, tea.Quit
			}
		}

	case responseMsg:
		m.isProcessing = false
		if msg.err != nil {
			m.addMessage("assistant", fmt.Sprintf("Error: %v", msg.err))
		} else {
			m.addMessage("assistant", msg.content)
		}
		m.updateView()

	case spinner.TickMsg:
		if m.isProcessing {
			s, cmd := m.spinner.Update(msg)
			m.spinner = s
			cmds = append(cmds, cmd)
		}
	}

	// Handle textarea input
	if !m.isProcessing {
		ta, cmd := m.textarea.Update(msg)
		m.textarea = ta
		cmds = append(cmds, cmd)
	}

	// Handle viewport scrolling
	vp, cmd := m.viewport.Update(msg)
	m.viewport = vp
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m SimpleModel) View() string {
	if !m.ready {
		return "\nInitializing..."
	}

	var b strings.Builder

	// Header
	header := fmt.Sprintf("Simple Agent Go | Model: %s | Provider: %s\nLoaded %d tools | Commands: /help, /tools, /clear, /exit\n",
		m.model, m.provider, m.toolCount)
	b.WriteString(header)
	b.WriteString(strings.Repeat("─", m.width) + "\n")

	// Conversation viewport
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Input area
	if m.isProcessing {
		b.WriteString(fmt.Sprintf("%s Processing...\n", m.spinner.View()))
	} else {
		b.WriteString(m.textarea.View())
	}

	return b.String()
}

// Helper methods

func (m *SimpleModel) handleInput(input string) {
	// Check for commands
	if strings.HasPrefix(input, "/") {
		switch strings.TrimSpace(input) {
		case "/help":
			m.addMessage("system", helpText)
			m.updateView()
			return
		case "/tools":
			m.addMessage("system", toolsText)
			m.updateView()
			return
		case "/clear":
			m.messages = []Message{}
			m.viewport.SetContent("")
			return
		case "/exit", "/quit":
			// This should trigger exit
			return
		}
	}

	// Check for bash commands
	if strings.HasPrefix(input, "!") {
		m.addMessage("user", input)
		m.addMessage("system", "Bash commands are not yet implemented")
		m.updateView()
		return
	}

	// Regular message
	m.addMessage("user", input)
	m.updateView()
}

func (m *SimpleModel) addMessage(role, content string) {
	m.messages = append(m.messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

func (m *SimpleModel) updateView() {
	var content strings.Builder

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			content.WriteString(fmt.Sprintf("\n> %s\n", msg.Content))
		case "assistant":
			content.WriteString(fmt.Sprintf("\n%s\n", msg.Content))
		case "system":
			content.WriteString(fmt.Sprintf("\n[%s]\n", msg.Content))
		}
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

func (m *SimpleModel) sendMessage(input string) tea.Cmd {
	m.isProcessing = true

	return func() tea.Msg {
		// Simulate API call for now
		time.Sleep(2 * time.Second)

		// In real implementation, call m.agent.Query(ctx, input)
		return responseMsg{
			content: "This is a simulated response. Real agent integration coming soon!",
		}
	}
}

// Message types
type responseMsg struct {
	content string
	err     error
}

// Help text
const helpText = `Available commands:
/help    - Show this help message
/tools   - List available tools
/model   - Change model (interactive)
/clear   - Clear conversation
/save    - Save conversation
/load    - Load conversation
/exit    - Exit application
!<cmd>   - Execute bash command`

const toolsText = `Loaded tools:
🧮 calculate - Evaluate mathematical expressions
📖 wikipedia - Search Wikipedia
🔍 google_search - Search the web
📄 read - Read file contents
💾 write - Write to files
📝 edit - Edit files
📁 directory_list - List directory contents`
