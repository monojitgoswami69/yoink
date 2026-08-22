package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the Yoink configuration.
type Config struct {
	LLMProvider string `json:"llm_provider"`
	LLMModel    string `json:"llm_model"`
	LLMAPIKey   string `json:"llm_api_key"`
	GitHubPAT   string `json:"github_pat,omitempty"`
}

// YoinkHome returns the platform-specific yoink home directory.
func YoinkHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, ".yoink"), nil
}

// ReposDir returns the directory where cloned repos live.
func ReposDir() (string, error) {
	home, err := YoinkHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "repos"), nil
}

// ConfigPath returns the path to the JSON config file.
func ConfigPath() (string, error) {
	home, err := YoinkHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.json"), nil
}

// Exists reports whether a configuration file is present.
func Exists() bool {
	p, err := ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// LoadOptional returns the config if present, or an empty config and no error.
// Use this when downstream code can run without an LLM configured (e.g. --no-agent).
func LoadOptional() *Config {
	cfg, err := load(false)
	if err != nil || cfg == nil {
		return &Config{}
	}
	return cfg
}

// Load reads the configuration and validates required fields.
func Load() (*Config, error) {
	return load(true)
}

func load(strict bool) (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if strict {
			return nil, fmt.Errorf("config not found at %s — run `yoink setup`", path)
		}
		return nil, nil
	}

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config file is not valid JSON: %w", err)
	}
	if strict {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// Validate checks that required fields are present (Ollama exempts API key).
func (c *Config) Validate() error {
	if c.LLMProvider == "" {
		return fmt.Errorf("config is missing llm_provider")
	}
	if c.LLMModel == "" {
		return fmt.Errorf("config is missing llm_model")
	}
	if c.LLMProvider != "ollama" && c.LLMAPIKey == "" {
		return fmt.Errorf("config is missing llm_api_key")
	}
	return nil
}

// Save writes the configuration to ~/.yoink/config.json with 0600 perms.
func Save(cfg *Config) error {
	home, err := YoinkHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return fmt.Errorf("failed to create yoink home: %w", err)
	}

	path := filepath.Join(home, "config.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}
