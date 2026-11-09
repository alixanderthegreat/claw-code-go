package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/runtime"
)

func TestShortcodeAt(t *testing.T) {
	for _, c := range []struct {
		before string
		emoji  string
		n      int
	}{
		{"done :checkered_flag:", "🏁", len(":checkered_flag:")},
		{":white_check_mark:", "✅", len(":white_check_mark:")},
		{":+1:", "👍", 4},
		{"meet at 12:30:", "", 0},       // not a name
		{"a:b:c:", "", 0},               // unknown
		{"::", "", 0},                   // empty name
		{"run `x :fire:", "", 0},        // inside inline code
		{"run `x` then :fire:", "🔥", 6}, // after it closed
		{":checkered_flag", "", 0},      // not closed yet
		{"see :Checkered_Flag:", "", 0}, // names are lower case
	} {
		e, n := shortcodeAt([]rune(c.before))
		if e != c.emoji || n != c.n {
			t.Errorf("shortcodeAt(%q) = %q, %d; want %q, %d", c.before, e, n, c.emoji, c.n)
		}
	}
}

func shortcodeModel() Model {
	cfg := &runtime.Config{Model: "m", MaxTokens: 100, ContextWindow: 32768}
	m := NewModel(cfg, runtime.NewConversationLoop(cfg, &recordingClient{}))
	m.width, m.height = 100, 30
	return m
}

func TestShortcodeTyped(t *testing.T) {
	m := typeText(shortcodeModel(), "ship it :checkered_flag: now")
	if got := m.textarea.Value(); got != "ship it 🏁 now" {
		t.Errorf("typed: got %q", got)
	}
}

func TestShortcodePasteStaysLiteral(t *testing.T) {
	mm, _ := shortcodeModel().Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x :fire:"), Paste: true})
	if got := mm.(Model).textarea.Value(); got != "x :fire:" {
		t.Errorf("pasted: got %q", got)
	}
}

func TestShortcodeSecondLine(t *testing.T) {
	m := shortcodeModel()
	m.textarea.SetValue("first line\nsecond ")
	m = typeText(m, ":fire:")
	if got := m.textarea.Value(); got != "first line\nsecond 🔥" {
		t.Errorf("second line: got %q", got)
	}
}
