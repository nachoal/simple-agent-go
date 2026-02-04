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

const (
	defaultBashTimeoutSecs = 30
	maxBashTimeoutSecs     = 300
)

type BashParams struct {
	Command string `json:"command" schema:"required" description:"Bash command to execute"`
	Timeout int    `json:"timeout,omitempty" description:"Timeout in seconds (optional, default 30)"`
}

// BashTool executes shell commands.
type BashTool struct {
	base.BaseTool
	allowedCommands []string
	allowAll        bool
}

// Parameters returns the parameters struct
func (t *BashTool) Parameters() interface{} {
	return &BashParams{}
}

// Execute runs a bash command.
func (t *BashTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args BashParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	command := strings.TrimSpace(args.Command)
	if command == "" {
		return "", NewToolError("VALIDATION_FAILED", "Command cannot be empty")
	}

	// Validate timeout
	timeout := args.Timeout
	if timeout <= 0 {
		timeout = defaultBashTimeoutSecs
	}
	if timeout < 1 || timeout > maxBashTimeoutSecs {
		timeout = defaultBashTimeoutSecs
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

func (t *BashTool) isCommandAllowed(cmd string) bool {
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
