package runtime

import (
	"claw-code-go/internal/config"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// LoadConfig reads configuration from layered settings files, the global
// config (~/.config/claw-code-go/config.json), and environment variables.
// Load order (later overrides earlier):
//  1. Defaults
//  2. Global config (~/.config/claw-code-go/config.json)
//  3. Environment variables
//  4. Layered settings files (user global → project → local)
//  5. CLI flags (applied by the caller after this function returns)
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

	// Apply global config as the base (~/.config/claw-code-go/config.json).
	if gc, err := config.LoadGlobal(); err == nil {
		if gc.Model != "" {
			cfg.Model = gc.Model
		}
		if gc.APIKey != "" {
			cfg.APIKey = gc.APIKey
		}
		if gc.BaseURL != "" {
			cfg.BaseURL = gc.BaseURL
		}
		if gc.MaxTokens != 0 {
			cfg.MaxTokens = gc.MaxTokens
		}
		if gc.ContextWindow != 0 {
			cfg.ContextWindow = gc.ContextWindow
		}
	}

	// Environment variables override global config.
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
	if ctxWindow := os.Getenv("CLAW_CONTEXT_WINDOW"); ctxWindow != "" {
		if n, err := strconv.Atoi(ctxWindow); err == nil && n > 0 {
			cfg.ContextWindow = n
		}
	}

	// Default session dir: ~/.claw-code/sessions
	homeDir, err := os.UserHomeDir()
	if err == nil {
		cfg.SessionDir = filepath.Join(homeDir, ".claw-code", "sessions")
	} else {
		cfg.SessionDir = ".claw-code-sessions"
	}

	// Detect the active provider from env vars and global config.
	cfg.ProviderName = detectProvider(cfg.BaseURL)

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

// detectProvider reads env vars and the resolved base URL to determine which
// provider to use. baseURL may come from OPENAI_BASE_URL or the global config
// file, so it's checked directly rather than re-reading the env var.
func detectProvider(baseURL string) string {
	switch {
	case os.Getenv("CLAUDE_CODE_USE_BEDROCK") == "1":
		return "bedrock"
	case os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1":
		return "vertex"
	case os.Getenv("CLAUDE_CODE_USE_FOUNDRY") == "1":
		return "foundry"
	case baseURL != "":
		return "openai"
	default:
		return "anthropic"
	}
}
