package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeText(m Model, s string) Model {
	for _, r := range s {
		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mm.(Model)
	}
	return m
}

// TestTypingDuringATurn: the input stays live while a reply streams - keys reach the box, and Alt+Enter
// queues the message instead of being dropped; it's sent the moment the turn ends.
func TestTypingDuringATurn(t *testing.T) {
	m := voiceModel("auto")
	m.history = newInputHistory()
	m.textarea.Focus() // as NewModel leaves it, for the whole session
	m.state = stateBusy

	m = typeText(m, "/voice off")
	if m.textarea.Value() != "/voice off" {
		t.Fatalf("typing during a turn reached the input as %q", m.textarea.Value())
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = mm.(Model)
	if m.queued != "/voice off" || m.textarea.Value() != "" || m.state != stateBusy {
		t.Fatalf("Alt+Enter during a turn: queued %q, input %q, state %v", m.queued, m.textarea.Value(), m.state)
	}

	// The turn ends: the queued command runs.
	mm, _ = m.Update(compactDoneMsg{summary: "s"})
	m = mm.(Model)
	if m.queued != "" || m.voice.mode != "off" {
		t.Errorf("after the turn: queued %q, voice mode %q - the queued /voice off never ran", m.queued, m.voice.mode)
	}
}
