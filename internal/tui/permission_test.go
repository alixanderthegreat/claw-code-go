package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TestPermissionShowsItAll: what's being allowed is readable to the end - it used to stop at 60
// characters, which cut a bash command, or a question, short - and the screen still fits.
func TestPermissionShowsItAll(t *testing.T) {
	m := voiceModel("off")
	m.width, m.height = 80, 24
	m.viewport = viewport.New(80, 16)
	m.permToolName = "bash"
	m.permToolInput = "cd /home/user/dev/play/context/simpleproject/cmd/simple-calendar && git push -u origin main && echo THE-END"
	view := m.viewPermissionOverlay()
	plain := strings.Join(strings.Fields(ansi.Strip(view)), " ")
	if !strings.Contains(plain, "THE-END") {
		t.Errorf("the command was cut short:\n%s", ansi.Strip(view))
	}
	if h := strings.Count(view, "\n") + 1; h > m.height {
		t.Errorf("overlay is %d lines on a %d-line screen", h, m.height)
	}
}

// permModel is a model asking permission for name with args, on a w×h screen.
func permModel(w, h int, name string, args map[string]any) Model {
	m := voiceModel("off")
	m.width, m.height = w, h
	m.viewport = viewport.New(w, h-8)
	m.viewport.SetContent(strings.Repeat("conversation\n", 50))
	m.permToolName, m.permArgs = name, args
	return m
}

// checkFits: the overlay is the screen's height, no line is wider than it, and the question and
// the keys are in view.
func checkFits(t *testing.T, m Model) string {
	t.Helper()
	view := ansi.Strip(m.viewPermissionOverlay())
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Errorf("overlay is %d lines on a %d-line screen", len(lines), m.height)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > m.width {
			t.Errorf("a line of %d columns on a %d-column screen: %q", w, m.width, l)
		}
	}
	for _, want := range []string{"Allow " + m.permToolName + "?", "[y]es once"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q isn't in view:\n%s", want, view)
		}
	}
	return view
}

// TestPermissionBoxSpansTheScreen: the box is as wide as the screen - it was 72 columns in the
// bottom-left corner - and a long command is there whole, past the 300 characters the summary had.
func TestPermissionBoxSpansTheScreen(t *testing.T) {
	cmd := "cd /home/user/dev && " + strings.Repeat("go test ./... && ", 30) + "echo THE-END"
	m := permModel(140, 40, "bash", map[string]any{"command": cmd})
	view := checkFits(t, m)
	if !strings.Contains(strings.Join(strings.Fields(view), " "), "echo THE-END") {
		t.Errorf("the command was cut:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if bottom := lines[len(lines)-1]; ansi.StringWidth(bottom) != 140 {
		t.Errorf("the box is %d columns on a 140-column screen", ansi.StringWidth(bottom))
	}
}

// TestPermissionBoxScrolls: a write taller than the screen scrolls inside the box, the keys stay
// in view, and the end can be reached.
func TestPermissionBoxScrolls(t *testing.T) {
	var content strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&content, "line %d of the file\n", i)
	}
	m := permModel(100, 30, "write_file", map[string]any{"path": "/tmp/big.txt", "content": content.String()})
	view := checkFits(t, m)
	if !strings.Contains(view, "line 1 of the file") || strings.Contains(view, "line 200 of") {
		t.Errorf("the top isn't what's shown first:\n%s", view)
	}
	if !strings.Contains(view, "of 202 · ↑/↓") {
		t.Errorf("no line saying where you are:\n%s", view)
	}
	mm, _ := m.handlePermissionKey(tea.KeyMsg{Type: tea.KeyEnd})
	m = mm.(Model)
	view = checkFits(t, m)
	if !strings.Contains(view, "line 200 of the file") {
		t.Errorf("End didn't reach the end:\n%s", view)
	}
	if m.permReplyCh != nil || m.state == stateBusy {
		t.Error("scrolling answered the question")
	}
}

// TestPermissionBoxShowsTheEdit: an edit shows its file and both sides, not just the path.
func TestPermissionBoxShowsTheEdit(t *testing.T) {
	m := permModel(100, 30, "file_edit", map[string]any{
		"file_path": "month.go", "old_string": "for col := 0; col < 7;\nday++", "new_string": "for col := startCol; col < 7;\nday++"})
	view := checkFits(t, m)
	for _, want := range []string{"month.go", "- for col := 0; col < 7;", "- day++", "+ for col := startCol; col < 7;", "+ day++"} {
		if !strings.Contains(view, want) {
			t.Errorf("%q isn't shown:\n%s", want, view)
		}
	}
}

// TestPermissionBoxSmallScreen: on a tiny screen the box still fits, with the keys in view.
func TestPermissionBoxSmallScreen(t *testing.T) {
	checkFits(t, permModel(40, 14, "bash", map[string]any{"command": strings.Repeat("echo hi; ", 60)}))
}
