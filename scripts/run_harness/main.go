package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/internal/userpaths"
)

const (
	defaultCheckTimeout = 15 * time.Minute
	maxCapturedOutput   = 16 * 1024
)

type HarnessResult struct {
	GeneratedAt        time.Time      `json:"generated_at"`
	RepoRoot           string         `json:"repo_root"`
	HarnessDir         string         `json:"harness_dir"`
	Mode               string         `json:"mode"`
	Summary            HarnessSummary `json:"summary"`
	CodexAnalysisDir   string         `json:"codex_analysis_dir,omitempty"`
	Checks             []CheckResult  `json:"checks"`
	PrivateAnalysisRun bool           `json:"private_analysis_run"`
	Comparison         *Comparison    `json:"comparison,omitempty"`
}

type HarnessSummary struct {
	Status           string   `json:"status"`
	TotalChecks      int      `json:"total_checks"`
	PassedChecks     int      `json:"passed_checks"`
	FailedChecks     int      `json:"failed_checks"`
	ScorePct         float64  `json:"score_pct"`
	TotalDurationMS  int64    `json:"total_duration_ms"`
	FailedCheckNames []string `json:"failed_check_names,omitempty"`
}

type CheckResult struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Comparison struct {
	PreviousRun string       `json:"previous_run,omitempty"`
	CheckDeltas []CheckDelta `json:"check_deltas,omitempty"`
}

type CheckDelta struct {
	Name           string `json:"name"`
	PreviousStatus string `json:"previous_status,omitempty"`
	CurrentStatus  string `json:"current_status"`
	PreviousMS     int64  `json:"previous_duration_ms,omitempty"`
	CurrentMS      int64  `json:"current_duration_ms"`
	DeltaMS        int64  `json:"delta_ms"`
}

func main() {
	repo := flag.String("repo", "", "Repository root to verify (default: current working directory)")
	skipCodex := flag.Bool("skip-codex-analysis", false, "Skip private Codex session analysis")
	mode := flag.String("mode", "private", "Harness mode: fast, public, or private")
	flag.Parse()

	repoRoot, err := resolveRepoRoot(*repo)
	if err != nil {
		fail(err)
	}

	harnessDir, err := userpaths.HarnessDir(repoRoot)
	if err != nil {
		fail(err)
	}

	result := HarnessResult{
		GeneratedAt: time.Now(),
		RepoRoot:    repoRoot,
		HarnessDir:  harnessDir,
		Mode:        strings.TrimSpace(*mode),
		Checks:      []CheckResult{},
	}

	checks := buildChecks(repoRoot, result.Mode)

	for _, check := range checks {
		res := runCommand(repoRoot, check.name, check.cmd...)
		result.Checks = append(result.Checks, res)
		if res.Status != "passed" {
			result.Summary = summarizeChecks(result.Checks)
			_ = writeHarnessResult(result)
			reportFailedCheck(res)
			fail(fmt.Errorf("%s failed", check.name))
		}
	}

	if result.Mode == "private" && !*skipCodex {
		result.PrivateAnalysisRun = true
		result.CodexAnalysisDir = filepath.Join(harnessDir, "codex-analysis")
		res := runCommand(repoRoot, "private-codex-analysis",
			"go", "run", "./scripts/analyze_codex_sessions",
			"--repo", repoRoot,
			"--out-dir", result.CodexAnalysisDir,
		)
		result.Checks = append(result.Checks, res)
		if res.Status != "passed" {
			result.Summary = summarizeChecks(result.Checks)
			_ = writeHarnessResult(result)
			reportFailedCheck(res)
			fail(fmt.Errorf("private codex analysis failed"))
		}
	}

	if result.Mode == "private" && liveCanariesEnabled() {
		canaryArgs := []string{"go", "run", "./scripts/run_live_canary", "--provider", "lmstudio"}
		if model := strings.TrimSpace(os.Getenv("LM_STUDIO_CANARY_MODEL")); model != "" {
			canaryArgs = append(canaryArgs, "--model", model)
		}
		if inferenceCanaryEnabled() {
			canaryArgs = append(canaryArgs, "--inference")
		}
		res := runCommand(repoRoot, "live-lmstudio-canary", canaryArgs...)
		result.Checks = append(result.Checks, res)
		if res.Status != "passed" {
			result.Summary = summarizeChecks(result.Checks)
			_ = writeHarnessResult(result)
			reportFailedCheck(res)
			fail(fmt.Errorf("live lmstudio canary failed"))
		}
	}

	result.Summary = summarizeChecks(result.Checks)
	if err := writeHarnessResult(result); err != nil {
		fail(err)
	}

	fmt.Printf("Harness complete for %s\n", repoRoot)
	fmt.Printf("Local harness dir: %s\n", harnessDir)
	fmt.Printf("Summary: %s (%d/%d checks, %.1f%%, %dms)\n",
		result.Summary.Status,
		result.Summary.PassedChecks,
		result.Summary.TotalChecks,
		result.Summary.ScorePct,
		result.Summary.TotalDurationMS,
	)
	if result.CodexAnalysisDir != "" {
		fmt.Printf("Private Codex analysis: %s\n", result.CodexAnalysisDir)
	}
	for _, check := range result.Checks {
		fmt.Printf("- %s: %s (%dms)\n", check.Name, check.Status, check.DurationMS)
	}
}

type harnessCheck struct {
	name string
	cmd  []string
}

func buildChecks(repoRoot, mode string) []harnessCheck {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "private"
	}

	goTestTarget := "./..."
	fixturesPath := "evals/public_fixtures.json"
	if mode == "fast" {
		if pkgs := changedGoPackages(repoRoot); len(pkgs) > 0 {
			goTestTarget = strings.Join(pkgs, " ")
		}
		fixturesPath = "evals/fast_fixtures.json"
	}

	return []harnessCheck{
		{name: "go-test", cmd: []string{"sh", "-c", "go test " + goTestTarget}},
		{name: "go-build", cmd: []string{"go", "build", "-o", "./simple-agent", "./cmd/simple-agent"}},
		{name: "public-evals", cmd: []string{"sh", "-c", "SIMPLE_AGENT_BINARY=./simple-agent go run ./scripts/run_public_evals --json --fixtures " + fixturesPath}},
		{name: "smoke-help", cmd: []string{"./simple-agent", "--help"}},
	}
}

func summarizeChecks(checks []CheckResult) HarnessSummary {
	summary := HarnessSummary{
		Status:           "passed",
		TotalChecks:      len(checks),
		FailedCheckNames: []string{},
	}

	for _, check := range checks {
		summary.TotalDurationMS += check.DurationMS
		if check.Status == "passed" {
			summary.PassedChecks++
			continue
		}
		summary.FailedChecks++
		summary.FailedCheckNames = append(summary.FailedCheckNames, check.Name)
	}

	if summary.TotalChecks > 0 {
		summary.ScorePct = float64(summary.PassedChecks) / float64(summary.TotalChecks) * 100
	}
	if summary.FailedChecks > 0 {
		summary.Status = "failed"
	}

	return summary
}

func liveCanariesEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("SIMPLE_AGENT_ENABLE_LIVE_CANARIES")))
	return v == "1" || v == "true" || v == "yes"
}

func inferenceCanaryEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("SIMPLE_AGENT_ENABLE_INFERENCE_CANARIES")))
	return v == "1" || v == "true" || v == "yes"
}

func resolveRepoRoot(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}
		input = wd
	}
	return filepath.Abs(input)
}

func runCommand(workdir, name string, argv ...string) CheckResult {
	start := time.Now()
	res := CheckResult{
		Name:    name,
		Command: strings.Join(argv, " "),
		Status:  "passed",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = workdir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()
	res.Output = truncateOutput(out.String())

	if ctx.Err() == context.DeadlineExceeded {
		res.Status = "failed"
		res.Error = "command timed out"
		return res
	}
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
	}
	return res
}

func changedGoPackages(repoRoot string) []string {
	packages := map[string]struct{}{}

	collect := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		out, err := cmd.Output()
		if err != nil {
			return
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			path := strings.TrimSpace(line)
			if path == "" || !strings.HasSuffix(path, ".go") {
				continue
			}
			dir := filepath.Dir(path)
			if dir == "." {
				continue
			}
			packages["./"+dir] = struct{}{}
		}
	}

	collect("diff", "--name-only", "--", "*.go")
	collect("diff", "--cached", "--name-only", "--", "*.go")
	collect("ls-files", "--others", "--exclude-standard", "--", "*.go")

	if len(packages) == 0 {
		return nil
	}

	outPkgs := make([]string, 0, len(packages))
	for pkg := range packages {
		outPkgs = append(outPkgs, pkg)
	}
	sort.Strings(outPkgs)
	return outPkgs
}

func writeHarnessResult(result HarnessResult) error {
	latestPath := filepath.Join(result.HarnessDir, "latest.json")
	if previous, err := loadPreviousHarnessResult(latestPath); err == nil {
		result.Comparison = compareResults(previous, result)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal harness result: %w", err)
	}

	if err := os.MkdirAll(result.HarnessDir, 0755); err != nil {
		return fmt.Errorf("failed to create harness dir %q: %w", result.HarnessDir, err)
	}

	if err := os.WriteFile(latestPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write %q: %w", latestPath, err)
	}

	runPath := filepath.Join(result.HarnessDir, fmt.Sprintf("run_%s.json", result.GeneratedAt.Format("20060102_150405")))
	if err := os.WriteFile(runPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write %q: %w", runPath, err)
	}

	return nil
}

func loadPreviousHarnessResult(path string) (HarnessResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HarnessResult{}, err
	}
	var result HarnessResult
	if err := json.Unmarshal(data, &result); err != nil {
		return HarnessResult{}, err
	}
	return result, nil
}

func compareResults(previous, current HarnessResult) *Comparison {
	index := map[string]CheckResult{}
	for _, check := range previous.Checks {
		index[check.Name] = check
	}

	deltas := make([]CheckDelta, 0, len(current.Checks))
	for _, check := range current.Checks {
		prev := index[check.Name]
		deltas = append(deltas, CheckDelta{
			Name:           check.Name,
			PreviousStatus: prev.Status,
			CurrentStatus:  check.Status,
			PreviousMS:     prev.DurationMS,
			CurrentMS:      check.DurationMS,
			DeltaMS:        check.DurationMS - prev.DurationMS,
		})
	}

	return &Comparison{
		PreviousRun: previous.GeneratedAt.Format(time.RFC3339),
		CheckDeltas: deltas,
	}
}

func truncateOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= maxCapturedOutput {
		return output
	}
	return output[:maxCapturedOutput] + "\n[output truncated]"
}

func reportFailedCheck(res CheckResult) {
	fmt.Fprintf(os.Stderr, "Check %s failed\n", res.Name)
	if strings.TrimSpace(res.Command) != "" {
		fmt.Fprintf(os.Stderr, "Command: %s\n", res.Command)
	}
	if strings.TrimSpace(res.Error) != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", res.Error)
	}
	if strings.TrimSpace(res.Output) != "" {
		fmt.Fprintf(os.Stderr, "Captured output:\n%s\n", res.Output)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
