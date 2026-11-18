package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/runtime"
)

// mcp.go: /mcp - the MCP servers Gordi uses: list them, add one to Gordi's config and connect it,
// remove one, or connect and disconnect for this session (greatproject #649).

const mcpUsage = "/mcp                          list the MCP servers and whether each is connected\n" +
	"/mcp add NAME COMMAND [ARGS...]  save a server to Gordi's config.json and connect it (a URL for COMMAND: an sse server)\n" +
	"/mcp remove NAME              disconnect it and take it out of the config\n" +
	"/mcp connect NAME             connect a configured server, for this session\n" +
	"/mcp disconnect NAME          drop it for this session; it's back next start"

// mcpConnectTimeout bounds starting a server and reading its tools; a language server's first
// start on a big repo is the slow case.
const mcpConnectTimeout = 2 * time.Minute

// mcpDoneMsg is a connect /mcp started, finished.
type mcpDoneMsg struct {
	name  string
	tools int
	err   error
}

func (m Model) handleMCPCommand(parts []string) (tea.Model, tea.Cmd) {
	sub := "list"
	if len(parts) > 1 {
		sub = parts[1]
	}
	name := ""
	if len(parts) > 2 {
		name = parts[2]
	}
	var cmd tea.Cmd
	switch {
	case sub == "list":
		m.viewBuf += renderBlock(statusStyle, mcpListText(m.loop.MCPStatus()))
	case sub == "add":
		srv, err := parseMCPAdd(parts[2:])
		if err == nil {
			err = m.loop.MCPAddConfig(srv)
		}
		if err != nil {
			m.viewBuf += renderBlock(errorStyle, "/mcp add: "+err.Error())
			break
		}
		m.viewBuf += renderBlock(statusStyle, fmt.Sprintf("Saved %q to Gordi's config.json; connecting...", srv.Name))
		cmd = mcpConnectCmd(m.loop, srv.Name)
	case sub == "remove" && name != "":
		if err := m.loop.MCPRemove(name); err != nil {
			m.viewBuf += renderBlock(errorStyle, "/mcp remove: "+err.Error())
			break
		}
		m.viewBuf += renderBlock(statusStyle, fmt.Sprintf("Removed %q: disconnected, and out of Gordi's config.json.", name))
	case sub == "connect" && name != "":
		m.viewBuf += renderBlock(statusStyle, fmt.Sprintf("Connecting %q...", name))
		cmd = mcpConnectCmd(m.loop, name)
	case sub == "disconnect" && name != "":
		if err := m.loop.MCPDisconnect(name); err != nil {
			m.viewBuf += renderBlock(errorStyle, "/mcp disconnect: "+err.Error())
			break
		}
		m.viewBuf += renderBlock(statusStyle, fmt.Sprintf("Disconnected %q for this session.", name))
	default:
		m.viewBuf += renderBlock(errorStyle, mcpUsage)
	}
	m = m.refreshViewport()
	return m, cmd
}

// mcpConnectCmd connects name off the UI's goroutine: a server can take seconds to start.
func mcpConnectCmd(loop *runtime.ConversationLoop, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mcpConnectTimeout)
		defer cancel()
		err := loop.MCPConnect(ctx, name)
		tools := 0
		for _, s := range loop.MCPStatus() {
			if s.Name == name {
				tools = s.Tools
			}
		}
		return mcpDoneMsg{name: name, tools: tools, err: err}
	}
}

// mcpDone reports a finished connect.
func (m Model) mcpDone(msg mcpDoneMsg) Model {
	if msg.err != nil {
		m.viewBuf += renderBlock(errorStyle, fmt.Sprintf("/mcp: %q didn't connect: %v (its log: ~/.claw-code/mcp.log)", msg.name, msg.err))
	} else {
		m.viewBuf += renderBlock(statusStyle, fmt.Sprintf("Connected %q: %d tools, offered from the next message.", msg.name, msg.tools))
	}
	return m.refreshViewport()
}

// parseMCPAdd is /mcp add's NAME COMMAND [ARGS...] as a server: a stdio one, with ~ expanded in
// the command and its arguments, or an sse one when COMMAND is a URL.
func parseMCPAdd(args []string) (runtime.MCPServerConfig, error) {
	if len(args) < 2 {
		return runtime.MCPServerConfig{}, fmt.Errorf("give a name and a command (or a URL)\n%s", mcpUsage)
	}
	name, target := args[0], args[1]
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if len(args) > 2 {
			return runtime.MCPServerConfig{}, fmt.Errorf("an sse server is a URL alone, with no arguments")
		}
		return runtime.MCPServerConfig{Name: name, Transport: "sse", URL: target}, nil
	}
	srv := runtime.MCPServerConfig{Name: name, Transport: "stdio", Command: expandHome(target)}
	for _, a := range args[2:] {
		srv.Args = append(srv.Args, expandHome(a))
	}
	return srv, nil
}

// expandHome is s with a leading ~ (alone, or ~/...) as the home directory: the server is started
// directly, not by a shell, so nothing else would expand it.
func expandHome(s string) string {
	if s != "~" && !strings.HasPrefix(s, "~/") {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	return filepath.Join(home, strings.TrimPrefix(s, "~"))
}

// mcpListText is /mcp's list: one line a server.
func mcpListText(servers []runtime.MCPServerStatus) string {
	if len(servers) == 0 {
		return "No MCP servers configured. /mcp add NAME COMMAND [ARGS...] adds one."
	}
	var b strings.Builder
	b.WriteString("MCP servers:")
	for _, s := range servers {
		state := "not connected"
		if s.Connected {
			state = fmt.Sprintf("connected, %d tools", s.Tools)
		}
		from := "config.json"
		if !s.Own {
			from = "~/.claude/settings.json or CLAUDE_MCP_SERVERS"
		}
		fmt.Fprintf(&b, "\n  %-12s %-22s %s  (%s)", s.Name, state, s.Where, from)
	}
	return b.String()
}
