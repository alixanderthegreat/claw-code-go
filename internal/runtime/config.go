package runtime

import (
	"claw-code-go/internal/config"
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DefaultModel     = "claude-sonnet-4-20250514"
	DefaultMaxTokens = 8096
	// DefaultContextWindow is a conservative fallback for the model's total
	// context length (input + output tokens). It's deliberately generic;
	// local/custom models should set CLAW_CONTEXT_WINDOW or context_window
	// in the global config to their real window size (e.g. 35000), since
	// compaction is measured against this value, not MaxTokens.
	DefaultContextWindow = 128000
)

// MCPServerConfig describes a single MCP server connection.
type MCPServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"` // "stdio" or "sse"
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// Config holds runtime configuration for the CLI.
type Config struct {
	Model        string
	MaxTokens    int
	SystemPrompt string
	SessionDir   string
	APIKey       string
	BaseURL      string

	// Provider and auth fields (Phase 3).
	// ProviderName is one of: "anthropic", "bedrock", "vertex", "foundry", "openai".
	ProviderName string
	// AuthMethod is one of: "api_key", "oauth", "iam", "adc", "azure_identity".
	AuthMethod string
	// OAuthToken is the resolved OAuth access token (set at startup when using OAuth).
	OAuthToken string

	// MCPServers lists MCP server connections (Phase 4).
	MCPServers []MCPServerConfig

	// Compaction settings (Phase 6).
	// CompactionEnabled enables automatic session compaction (default: true).
	CompactionEnabled bool
	// CompactionThreshold is the fraction of MaxTokens at which compaction
	// triggers (e.g., 0.75 triggers at 75% of the token budget).
	CompactionThreshold float64
	// CompactionKeepRecent is the number of most-recent messages retained
	// verbatim after compaction.
	CompactionKeepRecent int

	// Permission settings (Phase 11).
	// PermissionMode is the active permission enforcement mode string.
	PermissionMode string
	// AllowedTools are tool names that are always allowed without prompting.
	AllowedTools []string
	// BlockedTools are tool names that are always denied without prompting.
	BlockedTools []string

	// Theme is the active TUI color theme ("dark" or "light").
	Theme string

	// ContextWindow is the model's total context length in tokens (Phase 6).
	// Compaction is triggered as a fraction of this value, NOT of MaxTokens
	// (MaxTokens is only the per-request output cap sent to the API).
	ContextWindow int
}

// LoadConfig reads configuration from layered settings files and environment
// variables and applies defaults. Load order (later overrides earlier):
//  1. Defaults
//  2. Layered settings files (user global → project → local)
//  3. Environment variables
//  4. CLI flags (applied by the caller after this function returns)
func LoadConfig() *Config {
	cfg := &Config{
		Model:                DefaultModel,
		MaxTokens:            DefaultMaxTokens,
		PermissionMode:       "default",
		CompactionEnabled:    true,
		CompactionThreshold:  DefaultCompactionThreshold,
		CompactionKeepRecent: DefaultCompactionKeepRecent,
		ContextWindow:        DefaultContextWindow,
	}

	// Apply layered settings files (user global → project → local).
	s := config.Load()
	if s.Model != "" {
		cfg.Model = s.Model
	}
	if s.MaxTokens != 0 {
		cfg.MaxTokens = s.MaxTokens
	}
	if s.PermissionMode != "" {
		cfg.PermissionMode = s.PermissionMode
	}
	if len(s.AllowedTools) > 0 {
		cfg.AllowedTools = s.AllowedTools
	}
	if len(s.BlockedTools) > 0 {
		cfg.BlockedTools = s.BlockedTools
	}
	if s.Theme != "" {
		cfg.Theme = s.Theme
	}

	// Environment variables override settings files.
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.APIKey = key
	}
	if model := os.Getenv("ANTHROPIC_MODEL"); model != "" {
		cfg.Model = model
	}
	if baseURL := os.Getenv("ANTHROPIC_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	// Default session dir: ~/.claw-code/sessions
	homeDir, err := os.UserHomeDir()
	if err == nil {
		cfg.SessionDir = filepath.Join(homeDir, ".claw-code", "sessions")
	} else {
		cfg.SessionDir = ".claw-code-sessions"
	}

	// Detect the active provider from environment variables.
	// Note: If OPENAI_API_KEY is set, main.go will override ProviderName via
	// auth.ResolveCredentials() after LoadConfig returns.
	cfg.ProviderName = detectProvider()

	// Load MCP server configs.
	cfg.MCPServers = loadMCPServers(homeDir)

	return cfg
}

// loadMCPServers reads MCP server configurations from the settings file and
// the CLAUDE_MCP_SERVERS environment variable (JSON override, takes precedence).
func loadMCPServers(homeDir string) []MCPServerConfig {
	// Try env var override first.
	if raw := os.Getenv("CLAUDE_MCP_SERVERS"); raw != "" {
		var servers []MCPServerConfig
		if err := json.Unmarshal([]byte(raw), &servers); err == nil {
			return servers
		}
	}

	// Otherwise read from ~/.claude/settings.json.
	if homeDir == "" {
		return nil
	}
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings struct {
		MCPServers []MCPServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}

	return settings.MCPServers
}

// detectProvider reads env vars to determine which provider to use.
func detectProvider() string {
	switch {
	case os.Getenv("CLAUDE_CODE_USE_BEDROCK") == "1":
		return "bedrock"
	case os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1":
		return "vertex"
	case os.Getenv("CLAUDE_CODE_USE_FOUNDRY") == "1":
		return "foundry"
	case os.Getenv("OPENAI_BASE_URL") != "":
		return "openai"
	default:
		return "anthropic"
	}
}
