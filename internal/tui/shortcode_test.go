package tui

import (
	"strings"
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

func TestSignShortcodes(t *testing.T) {
	for code, want := range map[string]string{":stuck:": "🧱", ":blocked:": "🚧", ":why:": "🤔", ":idle:": "💤"} {
		if e, n := shortcodeAt([]rune("x " + code)); e != want || n != len(code) {
			t.Errorf("%s = %q, %d; want %q", code, e, n, want)
		}
	}
}

func TestSignPicker(t *testing.T) {
	m := shortcodeModel()
	press := func(k tea.KeyMsg) {
		mm, _ := m.Update(k)
		m = mm.(Model)
	}
	m = typeText(m, "night: ")
	press(tea.KeyMsg{Type: tea.KeyCtrlG})
	if !m.signs.open || !strings.Contains(m.renderInputArea(), ":done:") {
		t.Fatal("Ctrl+G didn't open the palette")
	}
	press(tea.KeyMsg{Type: tea.KeyRight}) // 🧱
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.signs.open || m.textarea.Value() != "night: 🧱" {
		t.Fatalf("Enter: open %v, value %q", m.signs.open, m.textarea.Value())
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlG})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}}) // ✅
	if m.textarea.Value() != "night: 🧱✅" {
		t.Errorf("key 7: value %q", m.textarea.Value())
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlG})
	press(tea.KeyMsg{Type: tea.KeyEsc})
	if m.signs.open || m.textarea.Value() != "night: 🧱✅" {
		t.Errorf("Esc: open %v, value %q", m.signs.open, m.textarea.Value())
	}
	// The palette takes a row of the input, not one more: the layout's height is the same.
	closed := strings.Count(m.renderInputArea(), "\n")
	press(tea.KeyMsg{Type: tea.KeyCtrlG})
	if open := strings.Count(m.renderInputArea(), "\n"); open != closed {
		t.Errorf("input area is %d lines open, %d closed", open+1, closed+1)
	}
}
