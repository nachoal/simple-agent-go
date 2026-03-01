package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nachoal/simple-agent-go/llm"
)

func TestRegistryReloadAndStaticModels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	content := `{
  "providers": {
    "lmstudio": {
      "baseUrl": "http://localhost:1234/v1",
      "api": "openai-completions",
      "models": [
        {"id": "qwen/test", "name": "Qwen Test", "input": ["text","image"], "maxTokens": 1234}
      ]
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	r := NewRegistry(path)
	if err := r.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	p, ok := r.Provider("lmstudio")
	if !ok {
		t.Fatalf("expected lmstudio provider")
	}
	if p.BaseURL != "http://localhost:1234/v1" {
		t.Fatalf("unexpected baseUrl: %q", p.BaseURL)
	}

	static := r.StaticModels()
	if len(static["lmstudio"]) != 1 {
		t.Fatalf("expected one static model, got %d", len(static["lmstudio"]))
	}
	model := static["lmstudio"][0]
	if model.ID != "qwen/test" {
		t.Fatalf("unexpected id: %q", model.ID)
	}
	if !model.SupportsVision {
		t.Fatalf("expected vision=true from input=image")
	}
	if model.MaxTokens != 1234 {
		t.Fatalf("unexpected maxTokens: %d", model.MaxTokens)
	}
}

func TestRegistryMergeLiveModels(t *testing.T) {
	r := NewRegistry("")
	r.providers["lmstudio"] = ProviderConfig{
		Name: "lmstudio",
		Models: []ModelDefinition{
			{ID: "qwen/test", Name: "Static Name", Input: []string{"text"}, MaxTokens: 2048},
		},
	}

	live := []llm.Model{
		{ID: "qwen/test", Description: "Live Name", MaxTokens: 1000},
		{ID: "live/only", Description: "Live Only"},
	}

	merged := r.MergeLiveModels("lmstudio", live)
	if len(merged) != 2 {
		t.Fatalf("expected 2 models, got %d", len(merged))
	}

	var foundStatic bool
	for _, model := range merged {
		if model.ID == "qwen/test" {
			foundStatic = true
			if model.Description != "Static Name" {
				t.Fatalf("expected static description override, got %q", model.Description)
			}
			if model.MaxTokens != 2048 {
				t.Fatalf("expected static maxTokens override, got %d", model.MaxTokens)
			}
		}
	}
	if !foundStatic {
		t.Fatalf("missing merged static model")
	}
}

func TestResolveConfigValueEnv(t *testing.T) {
	const key = "SIMPLE_AGENT_MODELS_REGISTRY_TEST"
	const want = "resolved-value"
	if err := os.Setenv(key, want); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(key)
	})

	got := ResolveConfigValue(key)
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
