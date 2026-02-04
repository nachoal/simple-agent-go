package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nachoal/simple-agent-go/tools/base"
)

func TestShellTool_AllowlistRejectsDisallowedCommand(t *testing.T) {
	tool := &BashTool{
		BaseTool: base.BaseTool{ToolName: "bash", ToolDesc: "test"},
		allowedCommands: []string{
			"echo",
		},
		allowAll: false,
	}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"curl https://example.com"}`))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	te, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T (%v)", err, err)
	}
	if te.Code != "COMMAND_NOT_ALLOWED" {
		t.Fatalf("expected COMMAND_NOT_ALLOWED, got %q (%v)", te.Code, te)
	}
}

func TestShellTool_YoloAllowsAnyCommand(t *testing.T) {
	tool := &BashTool{
		BaseTool:        base.BaseTool{ToolName: "bash", ToolDesc: "test"},
		allowedCommands: nil, // should be ignored when allowAll is true
		allowAll:        true,
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %T (%v)", err, err)
	}
	if !strings.Contains(out, "Exit Code: 0") {
		t.Fatalf("expected successful exit code in output, got:\n%s", out)
	}
}

func TestNewShellTool_YoloEnablesAllowAll(t *testing.T) {
	t.Setenv("SIMPLE_AGENT_YOLO", "true")

	tool := NewBashTool()
	shellTool, ok := tool.(*BashTool)
	if !ok {
		t.Fatalf("expected *BashTool, got %T", tool)
	}
	if !shellTool.allowAll {
		t.Fatalf("expected allowAll=true when SIMPLE_AGENT_YOLO is set")
	}
}
