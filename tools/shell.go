package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// ShellParams now uses generic input like Ruby
// The input can be either:
// 1. A simple command string
// 2. JSON with command and optional working_dir, timeout, env fields
type ShellParams = base.GenericParams

// ShellTool executes shell commands
type ShellTool struct {
	base.BaseTool
	allowedCommands []string
	allowAll        bool
}

// Parameters returns the parameters struct
func (t *ShellTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute runs a shell command
func (t *ShellTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// Parse input - can be either plain command or JSON with fields
	var command string
	var workingDir string
	var timeout int = 30 // default
	var env []string

	// Try to parse as JSON first
	var cmdParams struct {
		Command    string   `json:"command"`
		WorkingDir string   `json:"working_dir,omitempty"`
		Timeout    int      `json:"timeout,omitempty"`
		Env        []string `json:"env,omitempty"`
	}

	if err := json.Unmarshal([]byte(args.Input), &cmdParams); err == nil && cmdParams.Command != "" {
		// Successfully parsed as JSON
		command = cmdParams.Command
		workingDir = cmdParams.WorkingDir
		if cmdParams.Timeout > 0 {
			timeout = cmdParams.Timeout
		}
		env = cmdParams.Env
	} else {
		// Treat as plain command string
		command = strings.TrimSpace(args.Input)
	}

	if command == "" {
		return "", NewToolError("VALIDATION_FAILED", "Command cannot be empty")
	}

	// Validate timeout
	if timeout < 1 || timeout > 300 {
		timeout = 30
	}
	if timeout == 0 {
		timeout = 30
	}

	// Check if command is allowed (basic safety check)
	// In production, implement more sophisticated sandboxing
	baseCmd := strings.Fields(command)[0]
	if !t.allowAll && !t.isCommandAllowed(baseCmd) {
		return "", NewToolError("COMMAND_NOT_ALLOWED", "Command is not in the allowed list (start simple-agent with --yolo to allow any command)").
			WithDetail("command", baseCmd).
			WithDetail("allowed", strings.Join(t.allowedCommands, ", "))
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Determine shell based on OS
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}

	// Set working directory if specified
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Add environment variables
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	result := fmt.Sprintf("Command: %s\n", command)
	result += fmt.Sprintf("Duration: %v\n", duration)

	if workingDir != "" {
		result += fmt.Sprintf("Working Directory: %s\n", workingDir)
	}

	result += "\n"

	// Add stdout
	if stdout.Len() > 0 {
		result += "Output:\n"
		result += stdout.String()
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
	}

	// Add stderr if present
	if stderr.Len() > 0 {
		result += "\nError Output:\n"
		result += stderr.String()
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
	}

	// Check for errors
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return result + fmt.Sprintf("\nCommand timed out after %d seconds", timeout), nil
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			result += fmt.Sprintf("\nExit Code: %d", exitErr.ExitCode())
		} else {
			return "", NewToolError("EXECUTION_ERROR", "Failed to execute command").
				WithDetail("error", err.Error()).
				WithDetail("output", result)
		}
	} else {
		result += "\nExit Code: 0"
	}

	return result, nil
}

func (t *ShellTool) isCommandAllowed(cmd string) bool {
	// Remove any path components
	baseCmd := strings.TrimPrefix(cmd, "/")
	if idx := strings.LastIndex(baseCmd, "/"); idx >= 0 {
		baseCmd = baseCmd[idx+1:]
	}

	for _, allowed := range t.allowedCommands {
		if baseCmd == allowed {
			return true
		}
	}
	return false
}
