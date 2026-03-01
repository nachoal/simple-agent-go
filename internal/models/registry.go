package models

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nachoal/simple-agent-go/internal/userpaths"
	"github.com/nachoal/simple-agent-go/llm"
)

// ModelDefinition describes a model entry in models.json.
type ModelDefinition struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	Input         []string `json:"input,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
}

// ProviderConfig describes a provider entry in models.json.
type ProviderConfig struct {
	Name       string            `json:"-"`
	BaseURL    string            `json:"baseUrl,omitempty"`
	API        string            `json:"api,omitempty"`
	APIKey     string            `json:"apiKey,omitempty"`
	AuthHeader bool              `json:"authHeader,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Models     []ModelDefinition `json:"models,omitempty"`
}

type fileConfig struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

var builtInProviderNames = map[string]struct{}{
	"openai":     {},
	"anthropic":  {},
	"minmax":     {},
	"moonshot":   {},
	"deepseek":   {},
	"perplexity": {},
	"groq":       {},
	"lmstudio":   {},
	"lm-studio":  {},
	"ollama":     {},
}

// Registry loads and serves custom model/provider configuration.
type Registry struct {
	path string

	mu        sync.RWMutex
	providers map[string]ProviderConfig
	loadErr   error
}

// NewRegistry creates a models registry for a specific path.
func NewRegistry(path string) *Registry {
	return &Registry{
		path:      path,
		providers: make(map[string]ProviderConfig),
	}
}

// DefaultModelsPath returns ~/.simple-agent/agent/models.json.
func DefaultModelsPath() (string, error) {
	agentDir, err := userpaths.AgentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(agentDir, "models.json"), nil
}

// Reload reloads models.json from disk.
func (r *Registry) Reload() error {
	if r == nil {
		return nil
	}

	nextProviders := make(map[string]ProviderConfig)
	var loadErr error

	if r.path != "" {
		data, err := os.ReadFile(r.path)
		if err != nil {
			if !os.IsNotExist(err) {
				loadErr = fmt.Errorf("failed to read models config %q: %w", r.path, err)
			}
		} else {
			var cfg fileConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				loadErr = fmt.Errorf("failed to parse models config %q: %w", r.path, err)
			} else {
				for name, p := range cfg.Providers {
					normalized := NormalizeProvider(name)
					p.Name = normalized
					if err := validateProviderConfig(p); err != nil {
						loadErr = fmt.Errorf("invalid provider %q in %s: %w", name, r.path, err)
						break
					}
					nextProviders[normalized] = p
				}
			}
		}
	}

	r.mu.Lock()
	r.providers = nextProviders
	r.loadErr = loadErr
	r.mu.Unlock()

	return loadErr
}

// Error returns the most recent load error.
func (r *Registry) Error() error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadErr
}

// Providers returns all configured providers sorted by name.
func (r *Registry) Providers() []ProviderConfig {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ProviderConfig, 0, len(names))
	for _, name := range names {
		out = append(out, cloneProviderConfig(r.providers[name]))
	}
	return out
}

// Provider returns a provider config by name.
func (r *Registry) Provider(name string) (ProviderConfig, bool) {
	if r == nil {
		return ProviderConfig{}, false
	}
	normalized := NormalizeProvider(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[normalized]
	if !ok {
		return ProviderConfig{}, false
	}
	return cloneProviderConfig(p), true
}

// StaticModels returns models declared in models.json by provider.
func (r *Registry) StaticModels() map[string][]llm.Model {
	out := make(map[string][]llm.Model)
	for _, provider := range r.Providers() {
		if len(provider.Models) == 0 {
			continue
		}
		models := make([]llm.Model, 0, len(provider.Models))
		for _, m := range provider.Models {
			models = append(models, toLLMModel(provider.Name, m))
		}
		out[provider.Name] = models
	}
	return out
}

// MergeLiveModels merges API-discovered models with static models from models.json.
// Static models are upserted by ID and can enrich metadata such as max tokens.
func (r *Registry) MergeLiveModels(provider string, live []llm.Model) []llm.Model {
	normalized := NormalizeProvider(provider)

	index := make(map[string]llm.Model, len(live))
	for _, m := range live {
		index[m.ID] = m
	}

	cfg, ok := r.Provider(normalized)
	if ok {
		for _, staticDef := range cfg.Models {
			staticModel := toLLMModel(normalized, staticDef)
			if existing, exists := index[staticModel.ID]; exists {
				merged := existing
				if staticModel.Description != "" {
					merged.Description = staticModel.Description
				}
				if staticModel.MaxTokens > 0 {
					merged.MaxTokens = staticModel.MaxTokens
				}
				if staticModel.SupportsVision {
					merged.SupportsVision = true
				}
				index[staticModel.ID] = merged
			} else {
				index[staticModel.ID] = staticModel
			}
		}
	}

	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]llm.Model, 0, len(ids))
	for _, id := range ids {
		out = append(out, index[id])
	}
	return out
}

// NormalizeProvider normalizes provider names for map lookups.
func NormalizeProvider(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "lm-studio" {
		return "lmstudio"
	}
	return n
}

// IsBuiltInProvider reports whether a provider is built into simple-agent-go.
func IsBuiltInProvider(name string) bool {
	_, ok := builtInProviderNames[NormalizeProvider(name)]
	return ok
}

// ResolveConfigValue resolves config values from shell commands/env vars/literals.
func ResolveConfigValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, "!") {
		cmdText := strings.TrimSpace(strings.TrimPrefix(raw, "!"))
		if cmdText == "" {
			return ""
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "sh", "-lc", cmdText).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	if v := os.Getenv(raw); v != "" {
		return v
	}

	return raw
}

func validateProviderConfig(p ProviderConfig) error {
	if len(p.Models) > 0 && strings.TrimSpace(p.BaseURL) == "" {
		return fmt.Errorf("baseUrl is required when models are defined")
	}
	for _, m := range p.Models {
		if strings.TrimSpace(m.ID) == "" {
			return fmt.Errorf("model id cannot be empty")
		}
	}
	return nil
}

func cloneProviderConfig(in ProviderConfig) ProviderConfig {
	out := in
	if in.Headers != nil {
		out.Headers = make(map[string]string, len(in.Headers))
		for k, v := range in.Headers {
			out.Headers[k] = v
		}
	}
	if in.Models != nil {
		out.Models = make([]ModelDefinition, len(in.Models))
		copy(out.Models, in.Models)
	}
	return out
}

func toLLMModel(provider string, def ModelDefinition) llm.Model {
	description := def.Name
	if description == "" {
		description = "Configured via models.json"
	}
	return llm.Model{
		ID:             def.ID,
		Object:         "model",
		OwnedBy:        provider,
		MaxTokens:      def.MaxTokens,
		Description:    description,
		SupportsVision: hasImageInput(def),
	}
}

func hasImageInput(def ModelDefinition) bool {
	for _, v := range def.Input {
		if strings.EqualFold(strings.TrimSpace(v), "image") {
			return true
		}
	}

	id := strings.ToLower(def.ID)
	return strings.Contains(id, "vision") ||
		strings.Contains(id, "llava") ||
		strings.Contains(id, "pixtral") ||
		strings.Contains(id, "gemma-3")
}
