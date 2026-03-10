package tui

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/history"
	"github.com/nachoal/simple-agent-go/llm"
)

type blockingStreamAgent struct{}
type noopLLMClient struct{}

func (blockingStreamAgent) Query(context.Context, string) (*agent.Response, error) { return nil, nil }
func (blockingStreamAgent) QueryStream(context.Context, string) (<-chan agent.StreamEvent, error) {
	return make(chan agent.StreamEvent), nil
}
func (blockingStreamAgent) Clear()                                {}
func (blockingStreamAgent) GetMemory() []llm.Message              { return nil }
func (blockingStreamAgent) SetSystemPrompt(string)                {}
func (blockingStreamAgent) SetMemory([]llm.Message)               {}
func (blockingStreamAgent) SetRequestParams(agent.RequestParams)  {}
func (blockingStreamAgent) GetRequestParams() agent.RequestParams { return agent.RequestParams{} }

func (noopLLMClient) Chat(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, nil
}

func (noopLLMClient) ChatStream(context.Context, *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return make(chan llm.StreamEvent), nil
}

func (noopLLMClient) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }
func (noopLLMClient) GetModel(context.Context, string) (*llm.Model, error) {
	return nil, nil
}
func (noopLLMClient) Close() error { return nil }

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

func TestUpdateStreamingMessageDoesNotPanicOnModelCopy(t *testing.T) {
	ta := textarea.New()
	seed := "<think>\nThe user"
	m := BorderedTUI{
		textarea:         ta,
		streamingMessage: &llm.Message{Role: llm.RoleAssistant, Content: &seed},
		model:            "MiniMax-M2.5",
		provider:         "minmax",
		borderStyle:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic during streaming update: %v", r)
		}
	}()

	updatedText := "<think>\nThe user wants a greeting."
	updatedModel, _ := m.Update(toolEventMsg{
		event: agent.StreamEvent{
			Type: agent.EventTypeMessageUpdate,
			Message: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: &updatedText,
			},
		},
	})
	updated := updatedModel.(BorderedTUI)

	if updated.streamingMessage == nil || updated.streamingMessage.Content == nil {
		t.Fatalf("expected streaming message content to be set")
	}
	if got := *updated.streamingMessage.Content; got != "<think>\nThe user wants a greeting." {
		t.Fatalf("unexpected streamed content: %q", got)
	}

	view := stripANSI(updated.View())
	if !strings.Contains(view, "The user wants a greeting.") {
		t.Fatalf("expected view to include live streamed content, got: %q", view)
	}
}

func TestCanTypeWhileThinking(t *testing.T) {
	ta := textarea.New()
	ta.Focus()
	ta.SetValue("hello")

	m := BorderedTUI{
		textarea:    ta,
		model:       "MiniMax-M2.5",
		provider:    "minmax",
		borderStyle: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
	}

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := updatedModel.(BorderedTUI)
	if !updated.isThinking {
		t.Fatalf("expected model to enter thinking state after submit")
	}

	updatedModel, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	updated = updatedModel.(BorderedTUI)
	if got := updated.textarea.Value(); got != "n" {
		t.Fatalf("expected to type while thinking, got %q", got)
	}
}

func TestSendMessageReturnsOnCancelledContext(t *testing.T) {
	ta := textarea.New()
	m := BorderedTUI{
		agent:         blockingStreamAgent{},
		textarea:      ta,
		model:         "MiniMax-M2.5",
		provider:      "minmax",
		borderStyle:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
		toolEventChan: make(chan agent.StreamEvent, 1),
	}

	runCtx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := m.sendMessage(runCtx, "run-1", "hi")
	start := time.Now()
	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg on cancellation, got %T", msg)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("sendMessage should return quickly on cancellation, took %v", elapsed)
	}

	select {
	case _, ok := <-m.toolEventChan:
		if ok {
			t.Fatalf("expected tool event channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for tool event channel close")
	}
}

func TestSelectorConfirmPersistsSessionModelAndKeepsHistoryAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	historyMgr, err := history.NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	session, err := historyMgr.StartSession("/tmp/project", "lmstudio", "zai-org/glm-4.7-flash")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	system := "system"
	user := "who is she?"
	assistantMsg := "We were discussing a surgeon."
	baseMemory := []llm.Message{
		{Role: llm.RoleSystem, Content: &system},
		{Role: llm.RoleUser, Content: &user},
		{Role: llm.RoleAssistant, Content: &assistantMsg},
	}
	session.Messages = historyMgr.ConvertFromLLMMessages(baseMemory)
	if err := historyMgr.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	baseAgent := agent.New(noopLLMClient{}, agent.WithModel(session.Model), agent.WithSystemPrompt(system))
	baseAgent.SetMemory(baseMemory)
	historyAgent := agent.NewHistoryAgent(baseAgent, historyMgr, session)

	tuiModel := NewBorderedTUIWithHistory(noopLLMClient{}, historyAgent, session.Provider, session.Model, map[string]llm.Client{}, nil)
	tuiModel.SetClientFactory(func(providerName, modelName string) (llm.Client, error) {
		return noopLLMClient{}, nil
	})
	tuiModel.SetSystemPromptBuilder(func() string { return system })

	updatedModel, _ := tuiModel.Update(selectorConfirmMsg{provider: "ialab", model: "qwen-32b-dense"})
	var updated *BorderedTUI
	switch v := updatedModel.(type) {
	case *BorderedTUI:
		updated = v
	case BorderedTUI:
		updated = &v
	default:
		t.Fatalf("expected BorderedTUI, got %T", updatedModel)
	}

	switchedHistoryAgent, ok := updated.agent.(*agent.HistoryAgent)
	if !ok {
		t.Fatalf("expected history agent after switch, got %T", updated.agent)
	}

	gotMemory := switchedHistoryAgent.GetMemory()
	if len(gotMemory) != len(baseMemory) {
		t.Fatalf("expected memory length %d, got %d", len(baseMemory), len(gotMemory))
	}
	if gotMemory[len(gotMemory)-1].Content == nil || *gotMemory[len(gotMemory)-1].Content != assistantMsg {
		t.Fatalf("expected assistant history to persist, got %+v", gotMemory[len(gotMemory)-1])
	}

	loaded, err := historyMgr.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.Provider != "ialab" {
		t.Fatalf("expected persisted provider ialab, got %q", loaded.Provider)
	}
	if loaded.Model != "qwen-32b-dense" {
		t.Fatalf("expected persisted model qwen-32b-dense, got %q", loaded.Model)
	}
}

func TestResizedHistoryViewDoesNotDuplicateAssistantText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	historyMgr, err := history.NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	session, err := historyMgr.StartSession("/tmp/project", "ialab", "qwen3.5-27b-uncensored-q4")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	user := "hi"
	assistantMsg := "Hello! I can help with code changes, documentation updates, testing, or repository maintenance."
	session.Messages = historyMgr.ConvertFromLLMMessages([]llm.Message{
		{Role: llm.RoleUser, Content: &user},
		{Role: llm.RoleAssistant, Content: &assistantMsg},
	})
	if err := historyMgr.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	baseAgent := agent.New(noopLLMClient{}, agent.WithModel(session.Model))
	historyAgent := agent.NewHistoryAgent(baseAgent, historyMgr, session)
	tuiModel := NewBorderedTUIWithHistory(noopLLMClient{}, historyAgent, session.Provider, session.Model, map[string]llm.Client{}, nil)

	updatedModel, _ := tuiModel.Update(tea.WindowSizeMsg{Width: 48, Height: 18})
	updated := updatedModel.(BorderedTUI)
	view := normalizeWhitespace(stripANSI(updated.View()))
	if count := strings.Count(view, normalizeWhitespace(assistantMsg)); count != 1 {
		t.Fatalf("expected assistant history once after first resize, got %d in view: %q", count, view)
	}

	updatedModel, _ = updated.Update(tea.WindowSizeMsg{Width: 36, Height: 18})
	updated = updatedModel.(BorderedTUI)
	view = normalizeWhitespace(stripANSI(updated.View()))
	if count := strings.Count(view, normalizeWhitespace(assistantMsg)); count != 1 {
		t.Fatalf("expected assistant history once after second resize, got %d in view: %q", count, view)
	}
}

func TestStreamingCompletionDoesNotDuplicateAssistantTextAfterResize(t *testing.T) {
	ta := textarea.New()
	m := BorderedTUI{
		textarea:       ta,
		model:          "qwen3.5-27b-uncensored-q4",
		provider:       "ialab",
		borderStyle:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()),
		transcriptView: viewport.New(60, 10),
		transcript:     []transcriptEntry{},
	}
	m.syncLayout(true)

	start := "Hello"
	updatedModel, _ := m.Update(toolEventMsg{
		event: agent.StreamEvent{
			Type: agent.EventTypeMessageStart,
			Message: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: &start,
			},
		},
	})
	updated := updatedModel.(BorderedTUI)

	final := "Hello there from the streaming assistant."
	updatedModel, _ = updated.Update(toolEventMsg{
		event: agent.StreamEvent{
			Type: agent.EventTypeMessageEnd,
			Message: &llm.Message{
				Role:    llm.RoleAssistant,
				Content: &final,
			},
		},
	})
	updated = updatedModel.(BorderedTUI)

	view := normalizeWhitespace(stripANSI(updated.View()))
	if count := strings.Count(view, normalizeWhitespace(final)); count != 1 {
		t.Fatalf("expected final assistant text once before resize, got %d in view: %q", count, view)
	}

	updatedModel, _ = updated.Update(tea.WindowSizeMsg{Width: 42, Height: 16})
	updated = updatedModel.(BorderedTUI)
	view = normalizeWhitespace(stripANSI(updated.View()))
	if count := strings.Count(view, normalizeWhitespace(final)); count != 1 {
		t.Fatalf("expected final assistant text once after resize, got %d in view: %q", count, view)
	}
}
