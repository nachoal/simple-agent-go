package tui

import (
	"context"
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
	"github.com/nachoal/simple-agent-go/tui/styles"
)

// Theme type alias for convenience
type Theme = styles.Theme

// New creates a new TUI application
func New(llmClient llm.Client, agentInstance agent.Agent) *Model {
	// Initialize textarea
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false) // Enter sends message

	// Initialize viewports
	chatView := viewport.New(80, 20)
	toolsView := viewport.New(30, 20)

	// Get default theme
	theme := styles.GetTheme("default")

	return &Model{
		textarea:    ta,
		chatView:    chatView,
		toolsView:   toolsView,
		currentView: ViewChat,
		agent:       agentInstance,
		llmClient:   llmClient,
		messages:    []ChatMessage{},
		toolResults: make(map[string]string),
		showTools:   true,
		toolsWidth:  30,
		provider:    "openai",
		model:       "gpt-4",
		theme:       theme,
		keys:        DefaultKeyMap(),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.waitForActivity(),
	)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "ctrl+c", msg.String() == "ctrl+d":
			return m, tea.Quit

		case msg.String() == "ctrl+l":
			// Clear chat
			m.messages = []ChatMessage{}
			m.agent.Clear()
			m.updateChatView()
			return m, nil

		case msg.String() == "ctrl+t":
			// Toggle tools panel
			m.showTools = !m.showTools
			m.updateLayout()
			return m, nil

		case msg.String() == "enter" && !m.isProcessing:
			// Send message
			input := strings.TrimSpace(m.textarea.Value())
			if input != "" {
				m.textarea.SetValue("")
				cmds = append(cmds, m.sendMessage(input))
			}
			return m, tea.Batch(cmds...)

		case msg.String() == "tab":
			// Switch focus between panels
			if m.currentView == ViewChat {
				m.currentView = ViewTools
			} else {
				m.currentView = ViewChat
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		if !m.initialized {
			m.initialized = true
		}
		return m, nil

	case AgentResponseMsg:
		m.isProcessing = false
		if msg.Error != nil {
			m.error = msg.Error
			m.addMessage(ChatMessage{
				Role:      "error",
				Content:   fmt.Sprintf("Error: %v", msg.Error),
				Timestamp: time.Now(),
			})
		} else {
			m.addMessage(ChatMessage{
				Role:      "assistant",
				Content:   msg.Content,
				Timestamp: time.Now(),
				ToolCalls: msg.ToolCalls,
			})
		}
		m.updateChatView()
		return m, nil

	case StreamUpdateMsg:
		// Update the last assistant message with new content
		if len(m.messages) > 0 {
			lastIdx := len(m.messages) - 1
			if m.messages[lastIdx].Role == "assistant" {
				m.messages[lastIdx].Content += msg.Content
				m.updateChatView()
			}
		}
		return m, nil

	case ToolStartMsg:
		m.activeTools = append(m.activeTools, ToolExecution{
			ID:        msg.ID,
			Name:      msg.Name,
			StartTime: time.Now(),
			Status:    ToolStatusRunning,
		})
		m.updateToolsView()
		return m, nil

	case ToolCompleteMsg:
		// Update tool status
		for i, tool := range m.activeTools {
			if tool.ID == msg.ID {
				if msg.Error != nil {
					m.activeTools[i].Status = ToolStatusError
				} else {
					m.activeTools[i].Status = ToolStatusComplete
				}
				break
			}
		}
		m.toolResults[msg.ID] = msg.Result
		m.updateToolsView()
		return m, nil

	case TickMsg:
		// Update spinner or other animations
		return m, m.waitForActivity()
	}

	// Handle textarea updates
	if !m.isProcessing {
		ta, cmd := m.textarea.Update(msg)
		m.textarea = ta
		cmds = append(cmds, cmd)
	}

	// Handle viewport updates
	switch m.currentView {
	case ViewChat:
		vp, cmd := m.chatView.Update(msg)
		m.chatView = vp
		cmds = append(cmds, cmd)
	case ViewTools:
		vp, cmd := m.toolsView.Update(msg)
		m.toolsView = vp
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the application
func (m Model) View() string {
	if !m.initialized {
		return "Initializing..."
	}

	stylesInst := styles.NewStyles(m.theme)

	// Build the layout
	var sections []string

	// Header
	header := m.renderHeader(stylesInst)
	sections = append(sections, header)

	// Main content area
	mainContent := m.renderMainContent(stylesInst)
	sections = append(sections, mainContent)

	// Input area
	inputArea := m.renderInputArea(stylesInst)
	sections = append(sections, inputArea)

	// Footer
	footer := m.renderFooter(stylesInst)
	sections = append(sections, footer)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// Helper methods

func (m *Model) updateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Calculate dimensions
	headerHeight := 3
	footerHeight := 1
	inputHeight := 5
	mainHeight := m.height - headerHeight - footerHeight - inputHeight

	// Update chat view dimensions
	if m.showTools {
		chatWidth := m.width - m.toolsWidth - 3 // Account for borders and padding
		m.chatView.Width = chatWidth
		m.toolsView.Width = m.toolsWidth
	} else {
		m.chatView.Width = m.width - 2
	}

	m.chatView.Height = mainHeight - 2
	m.toolsView.Height = mainHeight - 2

	// Update textarea width
	m.textarea.SetWidth(m.width - 4)
}

func (m *Model) addMessage(msg ChatMessage) {
	m.messages = append(m.messages, msg)
}

func (m *Model) updateChatView() {
	stylesInst := styles.NewStyles(m.theme)
	
	var content strings.Builder
	for _, msg := range m.messages {
		// Render role
		role := stylesInst.RenderRole(msg.Role)
		content.WriteString(role)
		content.WriteString("\n")

		// Render content
		lines := strings.Split(msg.Content, "\n")
		for _, line := range lines {
			content.WriteString("  ")
			content.WriteString(line)
			content.WriteString("\n")
		}

		// Render tool calls if any
		if len(msg.ToolCalls) > 0 {
			content.WriteString("\n")
			for _, tool := range msg.ToolCalls {
				toolInfo := fmt.Sprintf("  🔧 %s", stylesInst.ToolName.Render(tool.Name))
				content.WriteString(toolInfo)
				content.WriteString("\n")
				if tool.Result != "" {
					content.WriteString(stylesInst.ToolResult.Render("  → " + tool.Result))
					content.WriteString("\n")
				}
			}
		}

		// Add timestamp
		timestamp := msg.Timestamp.Format("15:04:05")
		content.WriteString(stylesInst.Timestamp.Render("  " + timestamp))
		content.WriteString("\n\n")
	}

	m.chatView.SetContent(content.String())
	m.chatView.GotoBottom()
}

func (m *Model) updateToolsView() {
	stylesInst := styles.NewStyles(m.theme)
	
	var content strings.Builder
	content.WriteString(stylesInst.Title.Render("Active Tools"))
	content.WriteString("\n\n")

	for _, tool := range m.activeTools {
		// Tool name
		content.WriteString(stylesInst.ToolName.Render(tool.Name))
		content.WriteString("\n")

		// Tool status
		var status string
		switch tool.Status {
		case ToolStatusRunning:
			status = "running"
		case ToolStatusComplete:
			status = "success"
		case ToolStatusError:
			status = "error"
		default:
			status = "pending"
		}
		content.WriteString("  ")
		content.WriteString(stylesInst.RenderToolStatus(status))
		content.WriteString("\n")

		// Duration
		duration := time.Since(tool.StartTime).Round(time.Millisecond)
		content.WriteString("  ")
		content.WriteString(stylesInst.Label.Render(fmt.Sprintf("Duration: %v", duration)))
		content.WriteString("\n\n")
	}

	m.toolsView.SetContent(content.String())
}

func (m *Model) sendMessage(input string) tea.Cmd {
	m.isProcessing = true
	m.addMessage(ChatMessage{
		Role:      "user",
		Content:   input,
		Timestamp: time.Now(),
	})
	m.updateChatView()

	// Add a placeholder for assistant response
	m.addMessage(ChatMessage{
		Role:      "assistant",
		Content:   "",
		Timestamp: time.Now(),
	})

	return func() tea.Msg {
		ctx := context.Background()
		
		// Use streaming if available
		if m.agent != nil {
			events, err := m.agent.QueryStream(ctx, input)
			if err != nil {
				return AgentResponseMsg{Error: err}
			}

			var content strings.Builder
			var toolCalls []ToolCall

			for event := range events {
				switch event.Type {
				case agent.EventTypeMessage:
					content.WriteString(event.Content)
					// Send stream update
					// Note: In a real implementation, we'd use a channel to send these
				case agent.EventTypeToolStart:
					// Handle tool start
				case agent.EventTypeToolResult:
					// Handle tool result
				case agent.EventTypeError:
					return AgentResponseMsg{Error: event.Error}
				}
			}

			return AgentResponseMsg{
				Content:   content.String(),
				ToolCalls: toolCalls,
			}
		}

		// Fallback to non-streaming
		return AgentResponseMsg{
			Content: "Agent not initialized",
			Error:   fmt.Errorf("agent not initialized"),
		}
	}
}

func (m *Model) waitForActivity() tea.Cmd {
	return tea.Tick(time.Second/10, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m Model) renderHeader(s *styles.Styles) string {
	title := fmt.Sprintf("Simple Agent Go | %s | %s", m.provider, m.model)
	status := "◉ Connected"
	if m.isProcessing {
		status = "◎ Processing..."
	}
	if m.error != nil {
		status = "◈ Error"
	}

	left := s.Title.Render(title)
	right := s.Label.Render(status)
	
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 0 {
		gap = 0
	}

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		strings.Repeat(" ", gap),
		right,
	)

	return s.Header.Width(m.width).Render(header)
}

func (m Model) renderMainContent(s *styles.Styles) string {
	chatPanel := s.ChatPanel.
		Width(m.chatView.Width + 2).
		Height(m.chatView.Height + 2).
		Render(m.chatView.View())

	if !m.showTools {
		return chatPanel
	}

	toolsPanel := s.ToolsPanel.
		Width(m.toolsView.Width + 2).
		Height(m.toolsView.Height + 2).
		Render(m.toolsView.View())

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		chatPanel,
		toolsPanel,
	)
}

func (m Model) renderInputArea(s *styles.Styles) string {
	label := "Message:"
	if m.isProcessing {
		s := spinner.New()
		label = s.View() + " Processing..."
	}

	inputLabel := s.Label.Render(label)
	input := m.textarea.View()

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		inputLabel,
		input,
	)

	return s.InputArea.Width(m.width).Render(content)
}

func (m Model) renderFooter(s *styles.Styles) string {
	help := []string{
		"Enter: Send",
		"Ctrl+L: Clear",
		"Ctrl+T: Toggle Tools",
		"Ctrl+C: Quit",
	}

	helpText := strings.Join(help, " • ")
	return s.Footer.Width(m.width).Render(helpText)
}