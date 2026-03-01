package userpaths

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDirName = ".simple-agent"
	agentDirName  = "agent"
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
