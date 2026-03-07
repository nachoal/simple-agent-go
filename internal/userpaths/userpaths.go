package userpaths

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDirName  = ".simple-agent"
	agentDirName   = "agent"
	harnessDirName = "harness"
)

// ConfigDir returns ~/.simple-agent and ensures it exists.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}

	dir := filepath.Join(home, configDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}

	return dir, nil
}

// AgentDir returns ~/.simple-agent/agent and ensures it exists.
func AgentDir() (string, error) {
	configDir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(configDir, agentDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create agent directory %q: %w", dir, err)
	}

	return dir, nil
}

// HarnessDir returns ~/.simple-agent/harness/<repo-slug> and ensures it exists.
func HarnessDir(repoRoot string) (string, error) {
	configDir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	slug := repoSlug(repoRoot)
	dir := filepath.Join(configDir, harnessDirName, slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create harness directory %q: %w", dir, err)
	}

	return dir, nil
}

func repoSlug(repoRoot string) string {
	base := strings.ToLower(filepath.Base(filepath.Clean(repoRoot)))
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, string(filepath.Separator), "-")
	if base == "" || base == "." {
		base = "repo"
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(filepath.Clean(repoRoot)))
	return fmt.Sprintf("%s-%08x", base, hasher.Sum32())
}
