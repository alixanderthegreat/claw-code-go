package runtime

import "fmt"

// builtinTools are the tools ExecuteTool runs itself, whatever loop.Tools offers.
var builtinTools = map[string]bool{
	"bash": true, "read_file": true, "write_file": true, "glob": true, "grep": true,
	"file_edit": true, "web_fetch": true, "web_search": true, "ask_user": true, "todo_write": true,
}

// knownTool reports whether name is a tool this loop can run: a built-in, one it offers the model,
// or an MCP server's.
func (loop *ConversationLoop) knownTool(name string) bool {
	if builtinTools[name] {
		return true
	}
	for _, t := range loop.allTools() {
		if t.Name == name {
			return true
		}
	}
	if loop.MCPRegistry != nil {
		if _, _, ok := loop.MCPRegistry.FindTool(name); ok {
			return true
		}
	}
	return false
}

// unknownToolError is what the model hears back for a tool that doesn't exist. Models call a command
// on PATH by its name (a "consult" tool), so it says how to run one (greatproject #432).
func unknownToolError(name string) error {
	return fmt.Errorf("unknown tool: %s. If %s is a command on PATH, run it with the bash tool: "+
		"bash {\"command\": \"%s ...\"}", name, name, name)
}
