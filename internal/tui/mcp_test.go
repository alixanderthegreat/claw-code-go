package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claw-code-go/internal/runtime"
)

// TestParseMCPAdd: a command with its arguments, ~ expanded, is a stdio server; a URL alone is an
// sse one; too little, or a URL with arguments, is refused.
func TestParseMCPAdd(t *testing.T) {
	home, _ := os.UserHomeDir()
	srv, err := parseMCPAdd([]string{"serena", "~/bin/serena", "start-mcp-server", "--context", "~/ctx.yml", "--x=~"})
	if err != nil {
		t.Fatal(err)
	}
	if srv.Name != "serena" || srv.Transport != "stdio" || srv.Command != filepath.Join(home, "bin/serena") ||
		len(srv.Args) != 4 || srv.Args[2] != filepath.Join(home, "ctx.yml") || srv.Args[3] != "--x=~" {
		t.Errorf("stdio: %+v", srv)
	}
	srv, err = parseMCPAdd([]string{"remote", "https://example.test/sse"})
	if err != nil || srv.Transport != "sse" || srv.URL != "https://example.test/sse" || srv.Command != "" {
		t.Errorf("sse: %+v, %v", srv, err)
	}
	for _, bad := range [][]string{{}, {"onlyname"}, {"remote", "https://example.test/sse", "arg"}} {
		if _, err := parseMCPAdd(bad); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
}

// TestMCPListText: none configured says how to add one; each server says whether it's connected
// and where it's configured.
func TestMCPListText(t *testing.T) {
	if got := mcpListText(nil); !strings.Contains(got, "/mcp add") {
		t.Errorf("empty: %q", got)
	}
	got := mcpListText([]runtime.MCPServerStatus{
		{Name: "serena", Where: "/x/serena", Own: true, Connected: true, Tools: 10},
		{Name: "other", Where: "http://y", Own: false},
	})
	if !strings.Contains(got, "connected, 10 tools") || !strings.Contains(got, "(config.json)") ||
		!strings.Contains(got, "not connected") || !strings.Contains(got, "settings.json") {
		t.Errorf("list: %q", got)
	}
}
