package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"claw-code-go/internal/api"
)

// TestResumedTranscriptWraps: what's there before any new output - a resumed session - is wrapped
// as soon as the window's size is known, and again on a resize; it used to run off the edge.
func TestResumedTranscriptWraps(t *testing.T) {
	m := voiceModel("off")
	m.viewBuf = "You: " + strings.Repeat("let's consult some geniuses ", 20) + "THE END\n"
	for _, width := range []int{80, 50} {
		mm, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m = mm.(Model)
		view := ansi.Strip(m.viewport.View())
		if !strings.Contains(view, "THE END") {
			t.Errorf("width %d: the end of the line was cut off:\n%s", width, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Errorf("width %d: line of %d columns", width, ansi.StringWidth(line))
			}
		}
	}
}

// TestResumedConsultShownWhole: a consult in a resumed session reads as it did live - whole.
func TestResumedConsultShownWhole(t *testing.T) {
	msgs := []api.Message{
		{Role: "assistant", Content: []api.ContentBlock{{Type: "tool_use", ID: "c", Name: "consult", Input: map[string]any{"mode": "business", "question": "q"}}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "tool_result", ToolUseID: "c", Content: []api.ContentBlock{{Type: "text",
			Text: "business answered (~2.5k tokens, 77s). It saw only what you gave it.\n\n" + strings.Repeat("Any engineer can learn Kubernetes. ", 40) + "Final word."}}}}},
	}
	md := newMDStore()
	plain := strings.Join(strings.Fields(ansi.Strip(md.expand(renderHistory(msgs, md), 100))), " ") // wrapped, so whitespace-collapsed
	if !strings.Contains(plain, "Final word.") || strings.Contains(plain, "[Ctrl+O]") {
		t.Errorf("resumed consult cut short (%d bytes):\n%q", len(plain), plain[max(0, len(plain)-300):])
	}
}
