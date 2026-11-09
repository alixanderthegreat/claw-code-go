package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultModel is the model used when initializing global config without one.
	DefaultModel = "claude-sonnet-4-20250514"

	// templateContextWindow is the context size written into a fresh template.
	// Small on purpose: a local gateway's window is far below a frontier
	// model's, and an over-large value defeats compaction (see ContextWindow).
	templateContextWindow = 32768
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
	// Theme picks the TUI color scheme: "dark", "light", or "auto" (detect the
	// terminal's background). Empty means auto - so Gordi matches the terminal
	// instead of always starting dark.
	Theme string `json:"theme"`
	// VoiceMode turns voice on for this machine and says when the TUI speaks
	// replies: "auto" (when you spoke the message), "on" (always) or "off"
	// (you can still speak, nothing is said back). Empty means this machine
	// isn't set up for voice: no recording, nothing said - a gate, so a
	// machine without a mic or the speech endpoints doesn't offer it.
	VoiceMode string `json:"voice_mode"`
	// STTModel is the transcription model asked for; empty means small.en.
	STTModel string `json:"stt_model"`
	// SpeechVoice is the voice Gordi speaks in (the speech service's F1-F5, M1-M5). Empty is
	// the service's default, which is Mary's.
	SpeechVoice string `json:"speech_voice"`
	// SuggestNext shows the operator's likely next message as ghost text after each reply, Tab
	// to take it (#32). Off unless set.
	SuggestNext bool `json:"suggest_next"`
	// SuggestModel is the model the suggestion asks, when it isn't the main one: a small, fast
	// one is enough for one line from a short context. Empty is the main model.
	SuggestModel string `json:"suggest_model"`
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
// exist, writing a starter template the user fills in. Returns os.ErrExist if
// the file already exists.
//
// The file holds an api_key, so it is written 0600 in a 0700 directory. The
// template leaves base_url, model and api_key empty (not a frontier-model
// default): this fork talks to a local gateway, and an empty model is a clear
// "fill me in" rather than a wrong guess that fails confusingly at runtime.
func InitGlobal(model string) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return os.ErrExist
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	cfg := &GlobalConfig{
		Model:         model, // empty unless a caller insists
		ContextWindow: templateContextWindow,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// EnsureGlobal writes a starter template (via InitGlobal) when no global config
// exists yet. It reports whether it created one and the path, so a caller can
// tell the user what to fill in. An already-present file is left untouched.
func EnsureGlobal() (created bool, path string, err error) {
	path, err = GlobalConfigPath()
	if err != nil {
		return false, "", err
	}
	switch err := InitGlobal(""); {
	case err == nil:
		return true, path, nil
	case errors.Is(err, os.ErrExist):
		return false, path, nil
	default:
		return false, path, err
	}
}

// Validate reports a global config that exists but cannot be read or parsed,
// naming the file. A missing file is not an error (first run writes one). This
// is what turns a silent fallback-to-empty - where a typo looked exactly like
// "no config" - into a message that points at the actual file.
func Validate() error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("config %s is not valid JSON: %w", path, err)
	}
	return nil
}

// WorldReadable reports whether the config file is readable by group or other
// (any of 0o077) while holding an api_key - the condition a caller may warn or
// act on, the way ssh does for a private key.
func WorldReadable() (bool, os.FileMode, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return false, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}
	mode := info.Mode().Perm()
	cfg, err := LoadGlobal()
	if err != nil || cfg.APIKey == "" {
		return false, mode, nil
	}
	return mode&0o077 != 0, mode, nil
}
