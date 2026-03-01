package resources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderReloadContextOrderAndPrompts(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project", "subdir")
	agentDir := filepath.Join(root, ".simple-agent", "agent")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}

	writeFile := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	writeFile(filepath.Join(agentDir, "AGENTS.md"), "global context")
	writeFile(filepath.Join(root, "AGENTS.md"), "root context")
	writeFile(filepath.Join(root, "project", "CLAUDE.md"), "project context")
	writeFile(filepath.Join(agentDir, "prompts", "a.md"), "global prompt")
	writeFile(filepath.Join(project, ".simple-agent", "prompts", "b.md"), "local prompt")

	loader, err := NewLoader(project, agentDir)
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}

	snapshot := loader.Reload()
	if len(snapshot.ContextFiles) != 3 {
		t.Fatalf("expected 3 context files, got %d", len(snapshot.ContextFiles))
	}
	if snapshot.ContextFiles[0].Content != "global context" {
		t.Fatalf("unexpected context[0]: %q", snapshot.ContextFiles[0].Content)
	}
	if snapshot.ContextFiles[1].Content != "root context" {
		t.Fatalf("unexpected context[1]: %q", snapshot.ContextFiles[1].Content)
	}
	if snapshot.ContextFiles[2].Content != "project context" {
		t.Fatalf("unexpected context[2]: %q", snapshot.ContextFiles[2].Content)
	}

	if len(snapshot.PromptFragments) != 2 {
		t.Fatalf("expected 2 prompt fragments, got %d", len(snapshot.PromptFragments))
	}
}
