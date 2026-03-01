package improve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/agent"
)

const (
	defaultMaxChangedFiles = 24
	defaultCmdTimeout      = 10 * time.Minute
	maxCapturedOutput      = 16 * 1024
)

// VerificationResult stores one verification command result.
type VerificationResult struct {
	Command string
	Output  string
	Err     error
}

// Result is the outcome of an auto-improve run.
type Result struct {
	AgentSummary string
	ChangedFiles []string
	Verification []VerificationResult
}

// Runner coordinates a guarded "self-improve" execution.
type Runner struct {
	Cwd             string
	MaxChangedFiles int
}

// Enabled reports whether /improve is allowed in this process.
func Enabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("SIMPLE_AGENT_ENABLE_IMPROVE")))
	return value == "1" || value == "true" || value == "yes"
}

// NewRunner creates a runner for a working directory.
func NewRunner(cwd string) *Runner {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	return &Runner{
		Cwd:             cwd,
		MaxChangedFiles: defaultMaxChangedFiles,
	}
}

// Run executes one guarded auto-improve cycle.
func (r *Runner) Run(ctx context.Context, ag agent.Agent, goal string) (Result, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return Result{}, errors.New("improve goal cannot be empty")
	}
	if !Enabled() {
		return Result{}, errors.New("auto-improve is disabled; set SIMPLE_AGENT_ENABLE_IMPROVE=1 to enable")
	}
	if ag == nil {
		return Result{}, errors.New("agent is required")
	}

	prompt := buildImprovePrompt(goal)
	response, err := ag.Query(ctx, prompt)
	if err != nil {
		return Result{}, fmt.Errorf("self-improve query failed: %w", err)
	}

	result := Result{
		AgentSummary: strings.TrimSpace(response.Content),
	}

	changed, err := listChangedFiles(r.Cwd)
	if err != nil {
		return result, err
	}
	result.ChangedFiles = changed
	if len(changed) > r.MaxChangedFiles {
		return result, fmt.Errorf("safety stop: %d changed files exceeds limit %d", len(changed), r.MaxChangedFiles)
	}

	verification := []struct {
		command string
		args    []string
	}{
		{command: "go", args: []string{"test", "./..."}},
		{command: "go", args: []string{"build", "-o", "./simple-agent", "./cmd/simple-agent"}},
		{command: "./simple-agent", args: []string{"tools", "list"}},
	}
	for _, step := range verification {
		out, stepErr := runCommand(ctx, r.Cwd, step.command, step.args...)
		result.Verification = append(result.Verification, VerificationResult{
			Command: strings.Join(append([]string{step.command}, step.args...), " "),
			Output:  out,
			Err:     stepErr,
		})
		if stepErr != nil {
			return result, fmt.Errorf("verification failed for %q: %w", step.command, stepErr)
		}
	}

	return result, nil
}

func buildImprovePrompt(goal string) string {
	return strings.TrimSpace(fmt.Sprintf(
		`You are running a guarded self-improvement task for simple-agent-go.
Goal: %s

Constraints:
- Work only inside the current repository.
- Do not run git push, git reset --hard, or destructive commands.
- Keep changes focused and minimal.
- Prefer read/edit/write tools over shell.
- After edits, summarize exactly what changed and why.
`,
		goal,
	))
}

func listChangedFiles(cwd string) ([]string, error) {
	out, err := runCommand(context.Background(), cwd, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to inspect changed files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Porcelain format: XY<space>path
		if len(line) >= 4 {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}
	return files, nil
}

func runCommand(parent context.Context, cwd, command string, args ...string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	if !filepath.IsAbs(cwd) {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}

	timeout := defaultCmdTimeout
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cwd

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	output := out.String()
	if len(output) > maxCapturedOutput {
		output = output[:maxCapturedOutput] + "\n[output truncated]"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("command timed out")
	}
	return strings.TrimSpace(output), err
}
