package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"claw-code-go/internal/api"
)

// details.go: Ctrl+O, every tool call of the session in full. The transcript shows each call in a
// line - its input cut at 300 characters, its result at 500 - and a "…" there can't be opened, so a
// consultant's answer or a command's output past that was gone. This view has all of it.

// toolEntry is one tool call: what it was given and what it said.
type toolEntry struct {
	name   string
	args   map[string]any
	result string
	done   bool
}

// moreMark ends a line that was cut short, pointing at where the rest is.
const moreMark = " [Ctrl+O]"

// cut is truncate with moreMark when it cut anything.
func cut(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return truncate(s, n) + moreMark
}

// recordToolStart adds a call to the log.
func (m Model) recordToolStart(name string, args map[string]any) Model {
	m.tools = append(m.tools, toolEntry{name: name, args: args})
	return m
}

// recordToolDone fills in the latest unfinished call to name.
func (m Model) recordToolDone(name, result string) Model {
	for i := len(m.tools) - 1; i >= 0; i-- {
		if m.tools[i].name == name && !m.tools[i].done {
			m.tools[i].result, m.tools[i].done = result, true
			break
		}
	}
	return m
}

// toolsFromHistory rebuilds the log from a resumed session's messages.
func toolsFromHistory(messages []api.Message) []toolEntry {
	var out []toolEntry
	index := map[string]int{} // tool_use id -> entry
	for _, msg := range messages {
		for _, cb := range msg.Content {
			switch cb.Type {
			case "tool_use":
				index[cb.ID] = len(out)
				out = append(out, toolEntry{name: cb.Name, args: cb.Input})
			case "tool_result":
				if i, ok := index[cb.ToolUseID]; ok {
					var parts []string
					for _, in := range cb.Content {
						parts = append(parts, in.Text)
					}
					out[i].result, out[i].done = strings.Join(parts, "\n"), true
				}
			}
		}
	}
	return out
}

// openDetails shows the log, scrolled to the latest call.
func (m Model) openDetails() Model {
	m.details = true
	m.detailVP = viewport.New(m.width, max(m.height-2, 1))
	m.detailVP.SetContent(lipgloss.NewStyle().Width(max(m.width, 20)).Render(renderToolLog(m.tools)))
	m.detailVP.GotoBottom()
	return m
}

// handleDetailsKey: Esc, q or Ctrl+O close; the rest scrolls.
func (m Model) handleDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+o":
		m.details = false
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "home", "g":
		m.detailVP.GotoTop()
		return m, nil
	case "end", "G":
		m.detailVP.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.detailVP, cmd = m.detailVP.Update(msg)
	return m, cmd
}

func (m Model) viewDetails() string {
	title := headerStyle.Render("Tool calls") + statusStyle.Render(fmt.Sprintf("  %d this session  │  ↑↓ PgUp/PgDn scroll  │  Esc closes", len(m.tools)))
	return lipgloss.JoinVertical(lipgloss.Left, title, m.detailVP.View(), statusStyle.Render(fmt.Sprintf("%3.f%%", m.detailVP.ScrollPercent()*100)))
}

// renderToolLog is every call in full: each argument, then the whole result.
func renderToolLog(tools []toolEntry) string {
	if len(tools) == 0 {
		return statusStyle.Render("No tool calls yet in this session.")
	}
	var sb strings.Builder
	for i, t := range tools {
		sb.WriteString(toolRunningStyle.Render(fmt.Sprintf("◆ %d. %s", i+1, t.name)) + "\n")
		keys := make([]string, 0, len(t.args))
		for k := range t.args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(statusStyle.Render(k+":") + " " + argText(t.args[k]) + "\n")
		}
		switch {
		case !t.done:
			sb.WriteString(statusStyle.Render("(still running)") + "\n")
		case t.result == "":
			sb.WriteString(toolDoneStyle.Render("✓ (no output)") + "\n")
		default:
			sb.WriteString(toolDoneStyle.Render("✓ result:") + "\n" + t.result + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// argText is an argument as it was given: strings as they are (a file's content keeps its lines),
// anything else as JSON.
func argText(v any) string {
	if s, ok := v.(string); ok {
		if strings.Contains(s, "\n") {
			return "\n" + s
		}
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// consultDisplay is how a consult's result shows in the transcript: its header line, then the whole
// answer rendered as markdown - it's there to be read. ok is false for anything that isn't an answer.
func consultDisplay(result string, width int) (header, body string, ok bool) {
	head, answer, found := strings.Cut(result, "\n\n")
	if !found || strings.HasPrefix(result, "couldn't consult") {
		return "", "", false
	}
	return head, renderMarkdown(answer, width), true
}
