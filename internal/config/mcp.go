package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SaveMCPServers writes servers as mcp_servers in config.json, leaving every other key as it was
// (/mcp add and remove). The file holds the api_key, so it's replaced whole - written beside it
// and renamed - and stays 0600.
func SaveMCPServers(servers []MCPServer) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("%s isn't valid JSON, so it's left alone: %w", path, err)
		}
	case !os.IsNotExist(err):
		return err
	}
	if len(servers) == 0 {
		delete(raw, "mcp_servers")
	} else {
		list, err := json.Marshal(servers)
		if err != nil {
			return err
		}
		raw["mcp_servers"] = list
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone after the rename; cleans up if anything before it fails
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
