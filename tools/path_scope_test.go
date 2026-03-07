package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func expectOutsideWorkspaceError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected PATH_OUTSIDE_WORKSPACE error, got nil")
	}
	toolErr, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T (%v)", err, err)
	}
	if toolErr.Code != "PATH_OUTSIDE_WORKSPACE" {
		t.Fatalf("expected PATH_OUTSIDE_WORKSPACE, got %q", toolErr.Code)
	}
}

func TestWriteTool_BlocksPathsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	withWorkingDir(t, workspace)

	tool := NewWriteTool()
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"`+outside+`","content":"hello"}`))
	expectOutsideWorkspaceError(t, err)
}

func TestReadTool_BlocksPathsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	withWorkingDir(t, workspace)

	tool := NewReadTool()
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"`+outside+`"}`))
	expectOutsideWorkspaceError(t, err)
}

func TestEditTool_BlocksPathsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("before"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	withWorkingDir(t, workspace)

	tool := NewEditTool()
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"`+outside+`","oldText":"before","newText":"after"}`))
	expectOutsideWorkspaceError(t, err)
}

func TestDirectoryListTool_BlocksPathsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	withWorkingDir(t, workspace)

	tool := NewDirectoryListTool()
	raw := `{"input":"{\"path\":\"` + outside + `\"}"}`
	_, err := tool.Execute(context.Background(), json.RawMessage(raw))
	expectOutsideWorkspaceError(t, err)
}

func TestWriteTool_UsesWorkspaceRelativePath(t *testing.T) {
	workspace := t.TempDir()
	withWorkingDir(t, workspace)

	tool := NewWriteTool()
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"nested/file.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("write tool error: %v", err)
	}
	if !strings.Contains(out, "nested/file.txt") {
		t.Fatalf("expected relative path in output, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(workspace, "nested", "file.txt")); err != nil {
		t.Fatalf("expected file in workspace: %v", err)
	}
}
