package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the application configuration
type Config struct {
	DefaultProvider string `json:"default_provider"`
	DefaultModel    string `json:"default_model"`
}

// Manager handles configuration persistence
type Manager struct {
	configPath string
	config     *Config
}

// NewManager creates a new config manager
func NewManager() (*Manager, error) {
	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create config directory if it doesn't exist
	configDir := filepath.Join(homeDir, ".simple-agent")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.json")
	
	m := &Manager{
		configPath: configPath,
		config:     &Config{},
	}

	// Load existing config if it exists
	if err := m.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return m, nil
}

// Load reads the configuration from disk
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, m.config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	return nil
}

// Save writes the configuration to disk
func (m *Manager) Save() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// GetDefaultProvider returns the default provider
func (m *Manager) GetDefaultProvider() string {
	if m.config.DefaultProvider == "" {
		return "openai"
	}
	return m.config.DefaultProvider
}

// GetDefaultModel returns the default model
func (m *Manager) GetDefaultModel() string {
	return m.config.DefaultModel
}

// SetDefaults updates the default provider and model
func (m *Manager) SetDefaults(provider, model string) error {
	m.config.DefaultProvider = provider
	m.config.DefaultModel = model
	return m.Save()
}