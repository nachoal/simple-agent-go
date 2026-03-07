package history

import (
	"os"
	"path/filepath"
	"testing"
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
