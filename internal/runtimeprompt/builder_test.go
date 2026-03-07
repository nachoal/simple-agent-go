package runtimeprompt

import (
	"strings"
	"testing"

	"github.com/nachoal/simple-agent-go/internal/resources"
	"github.com/nachoal/simple-agent-go/internal/selfknowledge"
)

func TestBuild_IncludesCurrentWorkingDirectory(t *testing.T) {
	prompt := Build("base prompt", "/tmp/project", selfknowledge.Info{}, resources.Snapshot{})
	if !strings.Contains(prompt, "Current working directory:\n- /tmp/project") {
		t.Fatalf("expected cwd in prompt, got %q", prompt)
	}
}
