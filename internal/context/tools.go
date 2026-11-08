package context

import (
	"os"
	"os/exec"
	"strings"
)

// tools.go: the tools gate. The machine's GORDI.md (MachineFile) lists the command-line tools this
// machine has, under a "Tools" heading, one bullet each, the way a CLAUDE.md does:
//
//	## Tools on PATH
//
//	- `transcribe FILE [-name N]` - a recording as a timestamped transcript ...
//	- `speak "TEXT"` - says it aloud ...
//
// The model reads the list with the rest of GORDI.md. What the gate adds is the check: a tool the
// list names that isn't on PATH here is named to the model as missing, so a copied GORDI.md, or a
// tool not installed yet, doesn't have it calling something that isn't there.

// ToolNames is the commands a GORDI.md's Tools section lists, in order: each top-level bullet that
// opens with a backticked command gives that command's name, its first word. The section is a
// heading at any level whose text starts with "Tools", up to the next heading as high.
func ToolNames(md string) []string {
	var tools []string
	level := 0 // the Tools heading's level while inside its section, else 0
	for line := range strings.SplitSeq(md, "\n") {
		hashes := len(line) - len(strings.TrimLeft(line, "#"))
		if hashes > 0 && strings.HasPrefix(line[hashes:], " ") {
			switch {
			case strings.HasPrefix(strings.ToLower(strings.TrimSpace(line[hashes:])), "tools"):
				level = hashes
			case level > 0 && hashes <= level:
				level = 0 // a heading as high as the Tools one ends its section
			}
			continue
		}
		if level == 0 {
			continue
		}
		// A top-level bullet ("- " or "* " at the left edge) that opens with a backticked command.
		rest, ok := strings.CutPrefix(line, "- ")
		if !ok {
			if rest, ok = strings.CutPrefix(line, "* "); !ok {
				continue
			}
		}
		cmd, ok := strings.CutPrefix(rest, "`")
		if !ok {
			continue
		}
		cmd, _, _ = strings.Cut(cmd, "`")
		if f := strings.Fields(cmd); len(f) > 0 {
			tools = append(tools, f[0])
		}
	}
	return tools
}

// Tools is the machine's tools: each one its GORDI.md lists, and whether it's on PATH. Nil when
// there's no machine GORDI.md, or it lists none.
func Tools() []Tool {
	data, err := os.ReadFile(MachineFile())
	if err != nil {
		return nil
	}
	var tools []Tool
	for _, name := range ToolNames(string(data)) {
		_, err := exec.LookPath(name)
		tools = append(tools, Tool{Name: name, OnPath: err == nil})
	}
	return tools
}

// Tool is one tool the machine's GORDI.md lists.
type Tool struct {
	Name   string
	OnPath bool
}

// missingToolsNote tells the model which listed tools aren't on this machine, or is "" when none
// is missing.
func missingToolsNote() string {
	var missing []string
	for _, t := range Tools() {
		if !t.OnPath {
			missing = append(missing, "`"+t.Name+"`")
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "Not on this machine: " + strings.Join(missing, ", ") + ". The machine's GORDI.md lists them, " +
		"but they aren't on PATH here, so don't call them; say so if a task needs one."
}
