package styles

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme represents a color theme
type Theme struct {
	Name           string
	Primary        lipgloss.AdaptiveColor
	Secondary      lipgloss.AdaptiveColor
	Background     lipgloss.AdaptiveColor
	Surface        lipgloss.AdaptiveColor
	Text           lipgloss.AdaptiveColor
	TextDim        lipgloss.AdaptiveColor
	Border         lipgloss.AdaptiveColor
	Success        lipgloss.AdaptiveColor
	Warning        lipgloss.AdaptiveColor
	Error          lipgloss.AdaptiveColor
	Info           lipgloss.AdaptiveColor
	CodeBackground lipgloss.AdaptiveColor
}

// Default theme
var DefaultTheme = Theme{
	Name:           "default",
	Primary:        lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7B68EE"},
	Secondary:      lipgloss.AdaptiveColor{Light: "#6C6CFF", Dark: "#9370DB"},
	Background:     lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1E1E1E"},
	Surface:        lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#2D2D2D"},
	Text:           lipgloss.AdaptiveColor{Light: "#1E1E1E", Dark: "#E0E0E0"},
	TextDim:        lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"},
	Border:         lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#404040"},
	Success:        lipgloss.AdaptiveColor{Light: "#4CAF50", Dark: "#66BB6A"},
	Warning:        lipgloss.AdaptiveColor{Light: "#FF9800", Dark: "#FFA726"},
	Error:          lipgloss.AdaptiveColor{Light: "#F44336", Dark: "#EF5350"},
	Info:           lipgloss.AdaptiveColor{Light: "#2196F3", Dark: "#42A5F5"},
	CodeBackground: lipgloss.AdaptiveColor{Light: "#F5F5F5", Dark: "#1E1E1E"},
}

// Dracula theme
var DraculaTheme = Theme{
	Name:           "dracula",
	Primary:        lipgloss.AdaptiveColor{Light: "#BD93F9", Dark: "#BD93F9"},
	Secondary:      lipgloss.AdaptiveColor{Light: "#FF79C6", Dark: "#FF79C6"},
	Background:     lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#282A36"},
	Surface:        lipgloss.AdaptiveColor{Light: "#44475A", Dark: "#44475A"},
	Text:           lipgloss.AdaptiveColor{Light: "#F8F8F2", Dark: "#F8F8F2"},
	TextDim:        lipgloss.AdaptiveColor{Light: "#6272A4", Dark: "#6272A4"},
	Border:         lipgloss.AdaptiveColor{Light: "#6272A4", Dark: "#6272A4"},
	Success:        lipgloss.AdaptiveColor{Light: "#50FA7B", Dark: "#50FA7B"},
	Warning:        lipgloss.AdaptiveColor{Light: "#F1FA8C", Dark: "#F1FA8C"},
	Error:          lipgloss.AdaptiveColor{Light: "#FF5555", Dark: "#FF5555"},
	Info:           lipgloss.AdaptiveColor{Light: "#8BE9FD", Dark: "#8BE9FD"},
	CodeBackground: lipgloss.AdaptiveColor{Light: "#44475A", Dark: "#44475A"},
}

// Nord theme
var NordTheme = Theme{
	Name:           "nord",
	Primary:        lipgloss.AdaptiveColor{Light: "#5E81AC", Dark: "#81A1C1"},
	Secondary:      lipgloss.AdaptiveColor{Light: "#88C0D0", Dark: "#88C0D0"},
	Background:     lipgloss.AdaptiveColor{Light: "#2E3440", Dark: "#2E3440"},
	Surface:        lipgloss.AdaptiveColor{Light: "#3B4252", Dark: "#3B4252"},
	Text:           lipgloss.AdaptiveColor{Light: "#ECEFF4", Dark: "#D8DEE9"},
	TextDim:        lipgloss.AdaptiveColor{Light: "#4C566A", Dark: "#4C566A"},
	Border:         lipgloss.AdaptiveColor{Light: "#4C566A", Dark: "#4C566A"},
	Success:        lipgloss.AdaptiveColor{Light: "#A3BE8C", Dark: "#A3BE8C"},
	Warning:        lipgloss.AdaptiveColor{Light: "#EBCB8B", Dark: "#EBCB8B"},
	Error:          lipgloss.AdaptiveColor{Light: "#BF616A", Dark: "#BF616A"},
	Info:           lipgloss.AdaptiveColor{Light: "#5E81AC", Dark: "#81A1C1"},
	CodeBackground: lipgloss.AdaptiveColor{Light: "#3B4252", Dark: "#3B4252"},
}

// GetTheme returns a theme by name
func GetTheme(name string) Theme {
	switch name {
	case "dracula":
		return DraculaTheme
	case "nord":
		return NordTheme
	default:
		return DefaultTheme
	}
}
