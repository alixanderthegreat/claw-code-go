package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSaveMCPServers: mcp_servers is written and removed while every other key - the api_key
// above all - stays as it was, and the file stays 0600.
func TestSaveMCPServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "claw-code-go", "config.json")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(`{"api_key": "k", "model": "m", "voice_mode": "auto", "unknown_key": [1, 2]}`), 0o600)

	if err := SaveMCPServers([]MCPServer{{Name: "serena", Transport: "stdio", Command: "serena", Args: []string{"start-mcp-server"}}}); err != nil {
		t.Fatal(err)
	}
	gc, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if gc.APIKey != "k" || gc.Model != "m" || gc.VoiceMode != "auto" || len(gc.MCPServers) != 1 || gc.MCPServers[0].Args[0] != "start-mcp-server" {
		t.Errorf("after save: %+v", gc)
	}
	var raw map[string]json.RawMessage
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &raw)
	var unknown []int
	if json.Unmarshal(raw["unknown_key"], &unknown); len(unknown) != 2 || unknown[0] != 1 || unknown[1] != 2 {
		t.Errorf("a key Gordi doesn't know was changed: %s", raw["unknown_key"])
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}

	if err := SaveMCPServers(nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	raw = nil
	json.Unmarshal(b, &raw)
	if _, ok := raw["mcp_servers"]; ok || string(raw["api_key"]) != `"k"` {
		t.Errorf("after removing all: %s", b)
	}
}

// TestSaveMCPServersBadFile: a config that isn't valid JSON is left alone, not overwritten.
func TestSaveMCPServersBadFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "claw-code-go", "config.json")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(`{"api_key": "k",`), 0o600)
	if err := SaveMCPServers([]MCPServer{{Name: "x"}}); err == nil {
		t.Error("saved over a broken config")
	}
	if b, _ := os.ReadFile(path); string(b) != `{"api_key": "k",` {
		t.Errorf("the broken config changed: %q", b)
	}
}
