package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claw-code-go/internal/config"
)

// TestMCPAddAndRemoveConfig: a server added goes into Gordi's config.json and this session's
// config; a name already configured is refused; removing takes it out of both; one from
// ~/.claude/settings.json can't be removed here.
func TestMCPAddAndRemoveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "claw-code-go", "config.json")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(`{"api_key": "k"}`), 0o600)

	loop := &ConversationLoop{Config: &Config{MCPServers: []MCPServerConfig{{Name: "fromsettings", Transport: "sse", URL: "http://x"}}}}
	srv := MCPServerConfig{Name: "serena", Transport: "stdio", Command: "serena"}
	if err := loop.MCPAddConfig(srv); err != nil {
		t.Fatal(err)
	}
	if gc, _ := config.LoadGlobal(); len(gc.MCPServers) != 1 || gc.MCPServers[0].Name != "serena" || gc.APIKey != "k" {
		t.Errorf("config.json after add: %+v", gc)
	}
	if len(loop.Config.MCPServers) != 2 {
		t.Errorf("session config after add: %+v", loop.Config.MCPServers)
	}
	if err := loop.MCPAddConfig(srv); err == nil || !strings.Contains(err.Error(), "already configured") {
		t.Errorf("a second serena: %v", err)
	}

	st := loop.MCPStatus()
	if len(st) != 2 || st[0].Own || !st[1].Own || st[1].Connected {
		t.Errorf("status: %+v", st)
	}

	if err := loop.MCPRemove("fromsettings"); err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("removing one from settings.json: %v", err)
	}
	if err := loop.MCPRemove("serena"); err != nil {
		t.Fatal(err)
	}
	if gc, _ := config.LoadGlobal(); len(gc.MCPServers) != 0 || gc.APIKey != "k" {
		t.Errorf("config.json after remove: %+v", gc)
	}
	if len(loop.Config.MCPServers) != 1 || loop.Config.MCPServers[0].Name != "fromsettings" {
		t.Errorf("session config after remove: %+v", loop.Config.MCPServers)
	}
	if err := loop.MCPRemove("nothing"); err == nil {
		t.Error("removed a server that isn't configured")
	}
}
