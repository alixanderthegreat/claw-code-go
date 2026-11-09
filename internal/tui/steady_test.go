package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSteadyText(t *testing.T) {
	for in, want := range map[string]string{
		"plain text, é and 中文": "plain text, é and 中文",
		"ok 🤝 🏁":               "ok 🤝 🏁",
		"👨‍⚖️ rules":           ":man_judge: rules",
		"weigh ⚖️":             "weigh :scales:",
		"wave 👋🏻":              "wave :wave_tone1:",
	} {
		if got := steadyText(in); got != want {
			t.Errorf("steadyText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnsteadyShortcodeStaysTyped(t *testing.T) {
	if e, n := shortcodeAt([]rune("the :man_judge:")); n != 0 {
		t.Errorf(":man_judge: resolved to %q", e)
	}
	if e, _ := shortcodeAt([]rune(":checkered_flag:")); e != "🏁" {
		t.Errorf(":checkered_flag: = %q, want 🏁", e)
	}
}

func TestPastedJoinedEmojiGoesInAsShortcode(t *testing.T) {
	m := shortcodeModel()
	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ask the 👨‍⚖️"), Paste: true})
	if got := r.(Model).textarea.Value(); got != "ask the :man_judge:" {
		t.Errorf("input = %q", got)
	}
}

func TestWrapDrawsUnsteadyAsShortcode(t *testing.T) {
	if got := wrapAtSpaces("the 👨‍⚖️ said", 40); !strings.Contains(got, ":man_judge:") {
		t.Errorf("wrapAtSpaces = %q", got)
	}
}
