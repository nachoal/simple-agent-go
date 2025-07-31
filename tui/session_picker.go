package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/history"
)

// SessionPicker is a TUI component for selecting a conversation session
type SessionPicker struct {
	sessions          []history.SessionInfo
	selected          int
	done              bool
	width             int
	height            int
	SelectedSessionID string // The ID of the selected session
}

// SelectedSessionMsg is sent when a session is selected
type SelectedSessionMsg struct {
	SessionID string
}

// NewSessionPicker creates a new session picker
func NewSessionPicker(sessions []history.SessionInfo) *SessionPicker {
	return &SessionPicker{
		sessions: sessions,
		selected: 0,
		width:    80,
		height:   24,
	}
}

func (p SessionPicker) Init() tea.Cmd {
	return nil
}

func (p *SessionPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil
		
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.selected > 0 {
				p.selected--
			}
		case "down", "j":
			if p.selected < len(p.sessions)-1 {
				p.selected++
			}
		case "enter":
			if len(p.sessions) > 0 {
				p.SelectedSessionID = p.sessions[p.selected].ID
				return p, tea.Quit
			}
		case "esc", "q", "ctrl+c":
			return p, tea.Quit
		}
	}
	return p, nil
}

func (p SessionPicker) View() string {
	if len(p.sessions) == 0 {
		return "\nNo previous conversations found for this directory.\n\nPress [Esc] to start a new conversation."
	}
	
	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("75")).
		MarginBottom(1)
		
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("75")).
		Bold(true)
		
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("246"))
		
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		MarginTop(1)
	
	var b strings.Builder
	
	// Title
	b.WriteString(titleStyle.Render("Select a conversation to resume:"))
	b.WriteString("\n\n")
	
	// Calculate visible sessions based on height
	visibleHeight := p.height - 6 // Account for title, help, and margins
	startIdx := 0
	endIdx := len(p.sessions)
	
	if visibleHeight < len(p.sessions) {
		// Implement scrolling
		if p.selected > visibleHeight/2 {
			startIdx = p.selected - visibleHeight/2
			if startIdx+visibleHeight > len(p.sessions) {
				startIdx = len(p.sessions) - visibleHeight
			}
		}
		endIdx = startIdx + visibleHeight
		if endIdx > len(p.sessions) {
			endIdx = len(p.sessions)
		}
	}
	
	// Sessions
	for i := startIdx; i < endIdx; i++ {
		session := p.sessions[i]
		cursor := "  "
		style := normalStyle
		
		if i == p.selected {
			cursor = "▸ "
			style = selectedStyle
		}
		
		// Format session info
		line := fmt.Sprintf("%s%s - %s (%d messages, %s/%s)",
			cursor,
			session.CreatedAt.Format("Jan 02 15:04"),
			truncateString(session.Title, 40),
			session.Messages,
			session.Provider,
			session.Model)
			
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	
	// Scroll indicator
	if startIdx > 0 || endIdx < len(p.sessions) {
		scrollInfo := fmt.Sprintf("\n[%d-%d of %d sessions]", startIdx+1, endIdx, len(p.sessions))
		b.WriteString(normalStyle.Render(scrollInfo))
	}
	
	// Help
	help := "\n[↑/↓/j/k] Navigate  [Enter] Select  [Esc/q] Cancel"
	b.WriteString(helpStyle.Render(help))
	
	return b.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}