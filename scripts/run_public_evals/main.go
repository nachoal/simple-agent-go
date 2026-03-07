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
	"strings"
	"time"
)

const evalTimeout = 10 * time.Minute

type fixture struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Command     []string `json:"command"`
	ExpectJSON  bool     `json:"expect_json"`
}

type result struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Command     string `json:"command"`
	DurationMS  int64  `json:"duration_ms"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
}

func main() {
	jsonOutput := flag.Bool("json", false, "Emit JSON results")
	fixturesPath := flag.String("fixtures", "evals/public_fixtures.json", "Path to the public eval fixtures JSON")
	flag.Parse()

	data, err := os.ReadFile(*fixturesPath)
	if err != nil {
		fail(fmt.Errorf("failed to read fixtures: %w", err))
	}

	var fixtures []fixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		fail(fmt.Errorf("failed to parse fixtures: %w", err))
	}

	binary := os.Getenv("SIMPLE_AGENT_BINARY")
	if strings.TrimSpace(binary) == "" {
		binary = "./simple-agent"
	}

	results := make([]result, 0, len(fixtures))
	failed := false
	for _, fx := range fixtures {
		res := runFixture(fx, binary)
		results = append(results, res)
		if res.Status != "passed" {
			failed = true
		}
	}

	if *jsonOutput {
		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fail(err)
		}
		fmt.Println(string(out))
	} else {
		for _, res := range results {
			fmt.Printf("- %s: %s (%dms)\n", res.Name, res.Status, res.DurationMS)
			if res.Status != "passed" && res.Error != "" {
				fmt.Printf("  error: %s\n", res.Error)
			}
		}
	}

	if failed {
		os.Exit(1)
	}
}

func runFixture(fx fixture, binary string) result {
	args := make([]string, len(fx.Command))
	copy(args, fx.Command)
	for i, arg := range args {
		args[i] = strings.ReplaceAll(arg, "${BINARY}", binary)
	}

	res := result{
		Name:        fx.Name,
		Description: fx.Description,
		Command:     strings.Join(args, " "),
		Status:      "passed",
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = mustRepoRoot()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()
	res.Output = truncate(out.String(), 4096)

	if ctx.Err() == context.DeadlineExceeded {
		res.Status = "failed"
		res.Error = "timed out"
		return res
	}
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		return res
	}
	if fx.ExpectJSON {
		var parsed interface{}
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			res.Status = "failed"
			res.Error = fmt.Sprintf("expected JSON output: %v", err)
		}
	}
	return res
}

func mustRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Clean(wd)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
