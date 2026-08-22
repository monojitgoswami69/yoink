package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	in := &Config{LLMProvider: "openai", LLMModel: "gpt-4o", LLMAPIKey: "sk-test", GitHubPAT: "ghp_x"}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *in {
		t.Errorf("round-trip mismatch: got=%+v want=%+v", got, in)
	}

	path, _ := ConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("config perms = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadOptionalReturnsEmptyWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfg := LoadOptional()
	if cfg == nil {
		t.Fatal("LoadOptional returned nil")
	}
	if cfg.LLMProvider != "" {
		t.Errorf("expected empty provider, got %s", cfg.LLMProvider)
	}
}

func TestValidateRejectsMissingKey(t *testing.T) {
	c := &Config{LLMProvider: "openai", LLMModel: "gpt-4o"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected validate error when API key missing for non-ollama")
	}
}

func TestValidateAllowsOllamaWithoutKey(t *testing.T) {
	c := &Config{LLMProvider: "ollama", LLMModel: "llama3"}
	if err := c.Validate(); err != nil {
		t.Fatalf("ollama validate without key: %v", err)
	}
}

func TestSaveProducesValidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := Save(&Config{LLMProvider: "ollama", LLMModel: "llama3"}); err != nil {
		t.Fatal(err)
	}
	path, _ := ConfigPath()
	data, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("config not valid JSON: %v\n%s", err, data)
	}
}
