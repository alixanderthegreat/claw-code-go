package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// DefaultModel is the model used when initializing global config without one.
	DefaultModel = "claude-sonnet-4-20250514"
)

// GlobalConfig holds user-level defaults from ~/.config/claw-code-go/config.json.
// These are applied *before* layered settings files and env vars, so env vars
// and settings files still take final precedence.
type GlobalConfig struct {
	Model      string `json:"model"`
	APIKey     string `json:"api_key"`
	BaseURL    string `json:"base_url"`
	Provider   string `json:"provider"`
	MaxTokens  int    `json:"max_tokens"`
	Permission string `json:"permission_mode"`
	// ContextWindow is the model's total context length in tokens (input +
	// output combined). Unlike MaxTokens (the per-request output cap), this
	// is what compaction must be measured against. Required for local/custom
	// models with a small window (e.g. 35000) where the built-in default is
	// far too large to protect against overflowing the model's real limit.
	ContextWindow int `json:"context_window"`
}

// GlobalConfigPath returns the path to ~/.config/claw-code-go/config.json.
func GlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claw-code-go", "config.json"), nil
}

// LoadGlobal reads the global config file. Returns a zero-value GlobalConfig
// if the file does not exist or cannot be parsed (non-fatal).
func LoadGlobal() (*GlobalConfig, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return &GlobalConfig{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return &GlobalConfig{}, err
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Non-fatal: warn but continue with defaults.
		return &GlobalConfig{}, nil
	}

	return &cfg, nil
}

// InitGlobal creates ~/.config/claw-code-go/config.json if it doesn't already
// exist, writing a minimal default config. Returns os.ErrExist if the file
// already exists.
func InitGlobal(model string) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return os.ErrExist
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if model == "" {
		model = DefaultModel
	}
	cfg := &GlobalConfig{
		Model: model,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
