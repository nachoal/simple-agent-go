package tui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestViewDoesNotOverflowTerminalWidth(t *testing.T) {
	ta := textarea.New()
	ta.SetValue(strings.Repeat("x", 200))

	m := BorderedTUI{
		textarea:    ta,
		model:       "MiniMax-M2.5",
		provider:    "minmax",
		yoloEnabled: true,
		borderStyle: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
	}

	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	updated := updatedModel.(BorderedTUI)
	updated.adjustTextareaHeight()

	view := stripANSI(updated.View())
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if utf8.RuneCountInString(line) > 48 {
			t.Fatalf("line %d exceeds terminal width: %q", i+1, line)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	if got := truncateToWidth("abcdef", 4); got != "abc…" {
		t.Fatalf("unexpected truncate result: %q", got)
	}
	if got := truncateToWidth("a", 1); got != "a" {
		t.Fatalf("unexpected width-1 result: %q", got)
	}
}
