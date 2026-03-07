package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nachoal/simple-agent-go/tools/base"
)

func installStubCommand(t *testing.T, name string) {
	t.Helper()

	dir := t.TempDir()
	pathEnv := dir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", pathEnv)

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, name+".cmd")
		content := []byte("@echo off\r\necho stub instaloader\r\n")
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("write stub command: %v", err)
		}
		return
	}

	path := filepath.Join(dir, name)
	content := []byte("#!/bin/sh\nprintf 'stub instaloader\\n'\n")
	if err := os.WriteFile(path, content, 0755); err != nil {
		t.Fatalf("write stub command: %v", err)
	}
}

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

func TestBashTool_BlocksRiskyInstaloaderWithoutFailFastFlags(t *testing.T) {
	tool := &BashTool{
		BaseTool:        base.BaseTool{ToolName: "bash", ToolDesc: "test"},
		allowedCommands: nil,
		allowAll:        true,
	}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"instaloader --stories --highlights xiimenahm","timeout":30}`))
	if err == nil {
		t.Fatalf("expected risky command error, got nil")
	}

	te, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T (%v)", err, err)
	}
	if te.Code != "COMMAND_RISKY" {
		t.Fatalf("expected COMMAND_RISKY, got %q", te.Code)
	}
}

func TestBashTool_AllowsInstaloaderWithFailFastFlags(t *testing.T) {
	installStubCommand(t, "instaloader")

	tool := &BashTool{
		BaseTool:        base.BaseTool{ToolName: "bash", ToolDesc: "test"},
		allowedCommands: nil,
		allowAll:        true,
	}

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"instaloader --stories --highlights --max-connection-attempts 1 --abort-on 429 --help","timeout":30}`))
	if err != nil {
		t.Fatalf("expected nil error, got %T (%v)", err, err)
	}
	if !strings.Contains(out, "Exit Code: 0") {
		t.Fatalf("expected successful exit code in output, got:\n%s", out)
	}
	if !strings.Contains(out, "stub instaloader") {
		t.Fatalf("expected stub command output, got:\n%s", out)
	}
}

func TestBashTool_BlocksInteractiveTailFollow(t *testing.T) {
	tool := &BashTool{
		BaseTool:        base.BaseTool{ToolName: "bash", ToolDesc: "test"},
		allowedCommands: nil,
		allowAll:        true,
	}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"tail -f /tmp/example.log","timeout":30}`))
	if err == nil {
		t.Fatalf("expected interactive command error, got nil")
	}

	te, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T (%v)", err, err)
	}
	if te.Code != "COMMAND_INTERACTIVE" {
		t.Fatalf("expected COMMAND_INTERACTIVE, got %q", te.Code)
	}
}

func TestBashTool_BlocksInteractiveGitCommitWithoutMessage(t *testing.T) {
	tool := &BashTool{
		BaseTool:        base.BaseTool{ToolName: "bash", ToolDesc: "test"},
		allowedCommands: nil,
		allowAll:        true,
	}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"git commit","timeout":30}`))
	if err == nil {
		t.Fatalf("expected interactive git commit error, got nil")
	}

	te, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T (%v)", err, err)
	}
	if te.Code != "COMMAND_INTERACTIVE" {
		t.Fatalf("expected COMMAND_INTERACTIVE, got %q", te.Code)
	}
}
