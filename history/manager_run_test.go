package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerBeginAndFinishRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	session, err := mgr.StartSession("/tmp/project", "openai", "gpt-4")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := mgr.BeginRun(session, "run-1", "query", "hello", "/tmp/trace.jsonl"); err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if session.Metadata.LastRunStatus != RunStatusRunning {
		t.Fatalf("expected running status, got %q", session.Metadata.LastRunStatus)
	}
	if len(session.Runs) != 1 {
		t.Fatalf("expected one run, got %d", len(session.Runs))
	}

	if err := mgr.FinishRun(session, "run-1", RunStatusCompleted, nil); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if session.Metadata.LastRunStatus != RunStatusCompleted {
		t.Fatalf("expected completed status, got %q", session.Metadata.LastRunStatus)
	}
	if session.Runs[0].FinishedAt.IsZero() {
		t.Fatalf("expected finished_at to be set")
	}

	loaded, err := mgr.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(loaded.Runs) != 1 {
		t.Fatalf("expected persisted run, got %d", len(loaded.Runs))
	}
	if loaded.Runs[0].TracePath != "/tmp/trace.jsonl" {
		t.Fatalf("unexpected trace path: %q", loaded.Runs[0].TracePath)
	}

	if _, err := os.Stat(filepath.Join(home, ".simple-agent", "sessions", session.ID+".json")); err != nil {
		t.Fatalf("expected session file to exist: %v", err)
	}
}

func TestManagerStartSessionPersistsAndTracksLastSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	session, err := mgr.StartSession("/tmp/project", "openai", "gpt-4")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".simple-agent", "sessions", session.ID+".json")); err != nil {
		t.Fatalf("expected persisted session file: %v", err)
	}

	last, err := mgr.GetLastSession()
	if err != nil {
		t.Fatalf("GetLastSession: %v", err)
	}
	if last.ID != session.ID {
		t.Fatalf("expected last session %q, got %q", session.ID, last.ID)
	}
}

func TestManagerListSessionsSortsByUpdatedAtAcrossPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	mgr, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	first, err := mgr.StartSession("/tmp/project-a", "openai", "gpt-4")
	if err != nil {
		t.Fatalf("StartSession first: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	second, err := mgr.StartSession("/tmp/project-b", "anthropic", "claude-3")
	if err != nil {
		t.Fatalf("StartSession second: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	first.Metadata.Title = "Most recent"
	if err := mgr.SaveSession(first); err != nil {
		t.Fatalf("SaveSession first: %v", err)
	}

	sessions, err := mgr.ListSessions(0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != first.ID {
		t.Fatalf("expected first session to sort first, got %q", sessions[0].ID)
	}
	if sessions[0].Path != first.Path {
		t.Fatalf("expected first session path %q, got %q", first.Path, sessions[0].Path)
	}
	if sessions[1].ID != second.ID {
		t.Fatalf("expected second session second, got %q", sessions[1].ID)
	}
}

func TestManagerConvertToResumeMessagesFiltersToolTranscript(t *testing.T) {
	mgr := &Manager{}
	system := "system"
	user := "book her"
	assistant := "Here are her contact details."
	toolOutput := "raw search output"

	got := mgr.ConvertToResumeMessages([]Message{
		{Role: "system", Content: &system},
		{Role: "user", Content: &user},
		{Role: "assistant", Content: strPtr(""), ToolCalls: []ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: FunctionCall{
				Name:      "google_search",
				Arguments: `{"query":"ximena"}`,
			},
		}}},
		{Role: "tool", Content: &toolOutput, ToolCallID: "call-1"},
		{Role: "assistant", Content: &assistant},
	})

	if len(got) != 3 {
		t.Fatalf("expected 3 resume messages, got %d", len(got))
	}
	if got[0].Role != "system" || got[1].Role != "user" || got[2].Role != "assistant" {
		t.Fatalf("unexpected resume roles: %+v", got)
	}
	if got[2].Content == nil || *got[2].Content != assistant {
		t.Fatalf("unexpected assistant content: %+v", got[2])
	}
}

func strPtr(v string) *string {
	return &v
}
