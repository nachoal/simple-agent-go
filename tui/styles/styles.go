package styles

import (
	"github.com/charmbracelet/lipgloss"
)

// Styles holds all the styles for the application
type Styles struct {
	Theme Theme

	// Layout
	App          lipgloss.Style
	Header       lipgloss.Style
	Footer       lipgloss.Style
	ChatPanel    lipgloss.Style
	ToolsPanel   lipgloss.Style
	InputArea    lipgloss.Style

	// Messages
	UserMessage      lipgloss.Style
	AssistantMessage lipgloss.Style
	SystemMessage    lipgloss.Style
	ToolMessage      lipgloss.Style
	ErrorMessage     lipgloss.Style
	Timestamp        lipgloss.Style

	// Tools
	ToolName    lipgloss.Style
	ToolRunning lipgloss.Style
	ToolSuccess lipgloss.Style
	ToolError   lipgloss.Style
	ToolArgs    lipgloss.Style
	ToolResult  lipgloss.Style

	// UI Elements
	Border       lipgloss.Style
	Title        lipgloss.Style
	Subtitle     lipgloss.Style
	Label        lipgloss.Style
	Help         lipgloss.Style
	Spinner      lipgloss.Style
	ProgressBar  lipgloss.Style
	StatusBar    lipgloss.Style

	// Code
	CodeBlock    lipgloss.Style
	CodeInline   lipgloss.Style
	Syntax       lipgloss.Style
}

// NewStyles creates a new styles instance with the given theme
func NewStyles(theme Theme) *Styles {
	s := &Styles{
		Theme: theme,
	}

	// Layout styles
	s.App = lipgloss.NewStyle().
		Background(theme.Background)

	s.Header = lipgloss.NewStyle().
		Background(theme.Surface).
		Foreground(theme.Text).
		Padding(0, 2).
		Bold(true)

	s.Footer = lipgloss.NewStyle().
		Background(theme.Surface).
		Foreground(theme.TextDim).
		Padding(0, 2)

	s.ChatPanel = lipgloss.NewStyle().
		Background(theme.Background).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border)

	s.ToolsPanel = lipgloss.NewStyle().
		Background(theme.Surface).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border)

	s.InputArea = lipgloss.NewStyle().
		Background(theme.Surface).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Primary)

	// Message styles
	s.UserMessage = lipgloss.NewStyle().
		Foreground(theme.Primary).
		PaddingLeft(2).
		PaddingRight(2).
		MarginBottom(1)

	s.AssistantMessage = lipgloss.NewStyle().
		Foreground(theme.Text).
		PaddingLeft(2).
		PaddingRight(2).
		MarginBottom(1)

	s.SystemMessage = lipgloss.NewStyle().
		Foreground(theme.TextDim).
		Italic(true).
		PaddingLeft(2).
		PaddingRight(2).
		MarginBottom(1)

	s.ToolMessage = lipgloss.NewStyle().
		Foreground(theme.Info).
		PaddingLeft(2).
		PaddingRight(2).
		MarginBottom(1)

	s.ErrorMessage = lipgloss.NewStyle().
		Foreground(theme.Error).
		Bold(true).
		PaddingLeft(2).
		PaddingRight(2).
		MarginBottom(1)

	s.Timestamp = lipgloss.NewStyle().
		Foreground(theme.TextDim).
		Italic(true)

	// Tool styles
	s.ToolName = lipgloss.NewStyle().
		Foreground(theme.Secondary).
		Bold(true)

	s.ToolRunning = lipgloss.NewStyle().
		Foreground(theme.Warning)

	s.ToolSuccess = lipgloss.NewStyle().
		Foreground(theme.Success)

	s.ToolError = lipgloss.NewStyle().
		Foreground(theme.Error)

	s.ToolArgs = lipgloss.NewStyle().
		Foreground(theme.TextDim).
		Italic(true)

	s.ToolResult = lipgloss.NewStyle().
		Background(theme.Surface).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(theme.Border)

	// UI Element styles
	s.Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border)

	s.Title = lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true).
		MarginBottom(1)

	s.Subtitle = lipgloss.NewStyle().
		Foreground(theme.Secondary).
		MarginBottom(1)

	s.Label = lipgloss.NewStyle().
		Foreground(theme.TextDim)

	s.Help = lipgloss.NewStyle().
		Foreground(theme.TextDim).
		Italic(true)

	s.Spinner = lipgloss.NewStyle().
		Foreground(theme.Primary)

	s.ProgressBar = lipgloss.NewStyle().
		Foreground(theme.Primary).
		Background(theme.Surface)

	s.StatusBar = lipgloss.NewStyle().
		Background(theme.Surface).
		Foreground(theme.Text).
		Padding(0, 1)

	// Code styles
	s.CodeBlock = lipgloss.NewStyle().
		Background(theme.CodeBackground).
		Foreground(theme.Text).
		Padding(1).
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Border)

	s.CodeInline = lipgloss.NewStyle().
		Background(theme.CodeBackground).
		Foreground(theme.Secondary).
		Padding(0, 1)

	s.Syntax = lipgloss.NewStyle().
		Background(theme.CodeBackground)

	return s
}

// RenderRole returns a styled role prefix
func (s *Styles) RenderRole(role string) string {
	switch role {
	case "user":
		return s.UserMessage.Copy().Bold(true).Render("You:")
	case "assistant":
		return s.AssistantMessage.Copy().Bold(true).Render("Assistant:")
	case "system":
		return s.SystemMessage.Copy().Bold(true).Render("System:")
	case "tool":
		return s.ToolMessage.Copy().Bold(true).Render("Tool:")
	default:
		return s.Label.Render(role + ":")
	}
}

// RenderToolStatus returns a styled tool status
func (s *Styles) RenderToolStatus(status string) string {
	switch status {
	case "running":
		return s.ToolRunning.Render("● Running")
	case "success":
		return s.ToolSuccess.Render("✓ Complete")
	case "error":
		return s.ToolError.Render("✗ Error")
	default:
		return s.Label.Render("◌ Pending")
	}
}