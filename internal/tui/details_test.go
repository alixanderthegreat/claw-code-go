package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"claw-code-go/internal/api"
)

// TestToolDetailsHaveItAll: what the transcript cuts short - a long command, a long result - is
// whole in the Ctrl+O view, and the cut line says where to look.
func TestToolDetailsHaveItAll(t *testing.T) {
	m := voiceModel("off")
	m.width, m.height, m.state = 100, 30, stateBusy
	command := "go test ./... " + strings.Repeat("-run X ", 60)
	output := strings.Repeat("ok line\n", 200) + "THE END"

	mm, _ := m.Update(streamToolMsg{name: "bash", input: command[:300] + "...", args: map[string]any{"command": command}})
	m = mm.(Model)
	mm, _ = m.Update(streamToolDoneMsg{name: "bash", result: output})
	m = mm.(Model)
	if plain := ansi.Strip(m.streamBuf); strings.Count(plain, "[Ctrl+O]") != 2 || strings.Contains(plain, "THE END") {
		t.Errorf("transcript should cut both and say so:\n%s", plain)
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = mm.(Model)
	if !m.details {
		t.Fatal("Ctrl+O should open the details")
	}
	log := ansi.Strip(renderToolLog(m.tools))
	if !strings.Contains(log, strings.TrimSpace(command)) || !strings.Contains(log, "THE END") {
		t.Errorf("details missing the whole command or output")
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if mm.(Model).details {
		t.Errorf("Esc should close the details, not cancel the turn")
	}
}

// TestConsultAnswerShownWhole: a consultant's answer is there to be read - all of it, not 500
// characters of it.
func TestConsultAnswerShownWhole(t *testing.T) {
	m := voiceModel("off")
	m.width, m.state = 100, stateBusy
	answer := strings.Repeat("A considered sentence. ", 80) + "Final line."
	mm, _ := m.Update(streamToolDoneMsg{name: "consult", result: "house answered (~1.3k tokens, 41s). It saw only what you gave it.\n\n" + answer})
	plain := ansi.Strip(mm.(Model).md.expand(mm.(Model).streamBuf, 100))
	if !strings.Contains(plain, "Final line.") || strings.Contains(plain, "[Ctrl+O]") {
		t.Errorf("consult answer cut short:\n%s", plain)
	}
}

// TestToolsFromHistory: a resumed session's calls are in the details too, each with its result.
func TestToolsFromHistory(t *testing.T) {
	msgs := []api.Message{
		{Role: "assistant", Content: []api.ContentBlock{{Type: "tool_use", ID: "a", Name: "read_file", Input: map[string]any{"path": "x.go"}}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "tool_result", ToolUseID: "a", Content: []api.ContentBlock{{Type: "text", Text: "package x"}}}}},
	}
	got := toolsFromHistory(msgs)
	if len(got) != 1 || got[0].name != "read_file" || got[0].args["path"] != "x.go" || got[0].result != "package x" || !got[0].done {
		t.Errorf("history tools = %+v", got)
	}
}

// TestFailedToolMarked: a tool that failed shows ✗ live, as it does in a resumed transcript - a
// timed-out fetch used to show ✓.
func TestFailedToolMarked(t *testing.T) {
	m := voiceModel("off")
	m.width, m.state = 100, stateBusy
	mm, _ := m.Update(streamToolDoneMsg{name: "web_fetch", result: "Error: web_fetch: context deadline exceeded", failed: true})
	mm, _ = mm.(Model).Update(streamToolDoneMsg{name: "web_fetch", result: "URL: http://example.com"})
	plain := ansi.Strip(mm.(Model).streamBuf)
	if !strings.Contains(plain, "✗ web_fetch → Error") || !strings.Contains(plain, "✓ web_fetch → URL") {
		t.Errorf("marks:\n%s", plain)
	}
}
