package runtime

import (
	"fmt"

	"claw-code-go/internal/config"
)

// mcp_servers.go: what /mcp lists, adds and removes. Gordi's own servers live in its config.json
// (mcp_servers); ~/.claude/settings.json and CLAUDE_MCP_SERVERS can add more, and those are
// listed and connected here but changed where they're written.

// MCPServerStatus is one configured MCP server, as /mcp lists it.
type MCPServerStatus struct {
	Name      string
	Where     string // the command it starts, or the URL it reaches
	Own       bool   // in Gordi's config.json, so /mcp remove can take it out
	Connected bool
	Tools     int
}

// MCPStatus is every configured server, in config order, with whether it's connected now.
func (loop *ConversationLoop) MCPStatus() []MCPServerStatus {
	own := map[string]bool{}
	if gc, err := config.LoadGlobal(); err == nil {
		for _, s := range gc.MCPServers {
			own[s.Name] = true
		}
	}
	var out []MCPServerStatus
	for _, s := range loop.Config.MCPServers {
		st := MCPServerStatus{Name: s.Name, Where: s.Command, Own: own[s.Name]}
		if s.Transport != "stdio" {
			st.Where = s.URL
		}
		if loop.MCPRegistry != nil {
			for _, name := range loop.MCPRegistry.ServerNames() {
				if name == s.Name {
					st.Connected = true
					st.Tools = len(loop.MCPRegistry.ServerTools(name))
				}
			}
		}
		out = append(out, st)
	}
	return out
}

// MCPAddConfig saves srv to Gordi's config.json and to this session's config, so MCPConnect can
// start it. A name already configured, anywhere, is refused: two servers by one name would shadow
// each other's tools.
func (loop *ConversationLoop) MCPAddConfig(srv MCPServerConfig) error {
	if srv.Name == "" {
		return fmt.Errorf("a server needs a name")
	}
	for _, s := range loop.Config.MCPServers {
		if s.Name == srv.Name {
			return fmt.Errorf("%q is already configured: /mcp remove %s first", srv.Name, srv.Name)
		}
	}
	gc, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	own := append(gc.MCPServers, config.MCPServer(srv))
	if err := config.SaveMCPServers(own); err != nil {
		return err
	}
	loop.Config.MCPServers = append(loop.Config.MCPServers, srv)
	return nil
}

// MCPRemove disconnects name, if it's connected, and takes it out of Gordi's config.json and this
// session's config. A server from ~/.claude/settings.json or CLAUDE_MCP_SERVERS is refused: it
// would only come back next start.
func (loop *ConversationLoop) MCPRemove(name string) error {
	gc, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	var kept []config.MCPServer
	found := false
	for _, s := range gc.MCPServers {
		if s.Name == name {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found {
		for _, s := range loop.Config.MCPServers {
			if s.Name == name {
				return fmt.Errorf("%q isn't in Gordi's config.json: it comes from ~/.claude/settings.json or CLAUDE_MCP_SERVERS, so remove it there (/mcp disconnect %s drops it for this session)", name, name)
			}
		}
		return fmt.Errorf("no MCP server %q is configured", name)
	}
	if err := config.SaveMCPServers(kept); err != nil {
		return err
	}
	if loop.mcpConnected(name) {
		loop.MCPDisconnect(name) // it's out of the config either way; a close error doesn't undo that
	}
	var rest []MCPServerConfig
	for _, s := range loop.Config.MCPServers {
		if s.Name != name {
			rest = append(rest, s)
		}
	}
	loop.Config.MCPServers = rest
	return nil
}

func (loop *ConversationLoop) mcpConnected(name string) bool {
	if loop.MCPRegistry == nil {
		return false
	}
	for _, n := range loop.MCPRegistry.ServerNames() {
		if n == name {
			return true
		}
	}
	return false
}
