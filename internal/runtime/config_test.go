package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMCPServersFromGordiConfig: Gordi's own mcp_servers come first; ~/.claude/settings.json adds
// the ones whose names Gordi's config doesn't use; CLAUDE_MCP_SERVERS replaces both.
func TestMCPServersFromGordiConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_MCP_SERVERS", "")
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".config/claw-code-go/config.json", `{"mcp_servers": [{"name": "serena", "transport": "stdio", "command": "serena", "args": ["start-mcp-server"]}]}`)
	write(".claude/settings.json", `{"mcpServers": [{"name": "serena", "transport": "stdio", "command": "old"}, {"name": "other", "transport": "sse", "url": "http://x"}]}`)

	got := LoadConfig().MCPServers
	if len(got) != 2 || got[0].Name != "serena" || got[0].Command != "serena" || got[0].Args[0] != "start-mcp-server" || got[1].Name != "other" {
		t.Errorf("got %+v; want Gordi's serena, then settings' other", got)
	}

	t.Setenv("CLAUDE_MCP_SERVERS", `[{"name": "only", "transport": "stdio", "command": "x"}]`)
	if got := LoadConfig().MCPServers; len(got) != 1 || got[0].Name != "only" {
		t.Errorf("with CLAUDE_MCP_SERVERS: got %+v; want only that", got)
	}
}
