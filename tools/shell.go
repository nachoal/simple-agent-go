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

// ShellParams defines the parameters for the shell tool
type ShellParams struct {
	Command    string   `json:"command" schema:"required" description:"Shell command to execute"`
	WorkingDir string   `json:"working_dir,omitempty" description:"Working directory for the command"`
	Timeout    int      `json:"timeout,omitempty" schema:"min:1,max:300" description:"Timeout in seconds (default: 30)"`
	Env        []string `json:"env,omitempty" description:"Additional environment variables (KEY=value format)"`
}

// ShellTool executes shell commands
type ShellTool struct {
	base.BaseTool
	allowedCommands []string
}


// Parameters returns the parameters struct
func (t *ShellTool) Parameters() interface{} {
	return &ShellParams{}
}

// Execute runs a shell command
func (t *ShellTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args ShellParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	if err := Validate(&args); err != nil {
		return "", NewToolError("VALIDATION_FAILED", "Parameter validation failed").
			WithDetail("error", err.Error())
	}

	// Set default timeout
	timeout := args.Timeout
	if timeout == 0 {
		timeout = 30
	}

	// Check if command is allowed (basic safety check)
	// In production, implement more sophisticated sandboxing
	baseCmd := strings.Fields(args.Command)[0]
	if !t.isCommandAllowed(baseCmd) {
		return "", NewToolError("COMMAND_NOT_ALLOWED", "Command is not in the allowed list").
			WithDetail("command", baseCmd).
			WithDetail("allowed", strings.Join(t.allowedCommands, ", "))
	}

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Determine shell based on OS
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "cmd", "/C", args.Command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", args.Command)
	}

	// Set working directory if specified
	if args.WorkingDir != "" {
		cmd.Dir = args.WorkingDir
	}

	// Add environment variables
	if len(args.Env) > 0 {
		cmd.Env = append(cmd.Environ(), args.Env...)
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
	result := fmt.Sprintf("Command: %s\n", args.Command)
	result += fmt.Sprintf("Duration: %v\n", duration)
	
	if args.WorkingDir != "" {
		result += fmt.Sprintf("Working Directory: %s\n", args.WorkingDir)
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

