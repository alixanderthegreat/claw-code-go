package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func press(m Model, keys ...tea.KeyType) Model {
	for _, k := range keys {
		mm, _ := m.Update(tea.KeyMsg{Type: k})
		m = mm.(Model)
	}
	return m
}

// TestHistoryThroughMultiLineEntries: ↑ and ↓ keep stepping through history
// when an entry has a Ctrl+N newline in it - they used to stop there, moving
// only the cursor inside it.
func TestHistoryThroughMultiLineEntries(t *testing.T) {
	m := voiceModel("off")
	m.history = newInputHistory()
	m.textarea.Focus() // as it is when typing; an unfocused textarea ignores keys
	for _, e := range []string{"first", "two\nlines", "third"} {
		m.history.Push(e)
	}
	m.textarea.SetValue("my draft")

	for i, want := range []string{"third", "two\nlines", "first", "first"} {
		if m = press(m, tea.KeyUp); m.textarea.Value() != want {
			t.Fatalf("↑ %d: %q, want %q", i+1, m.textarea.Value(), want)
		}
	}
	for i, want := range []string{"two\nlines", "third", "my draft"} {
		if m = press(m, tea.KeyDown); m.textarea.Value() != want {
			t.Fatalf("↓ %d: %q, want %q", i+1, m.textarea.Value(), want)
		}
	}
	// Past the draft, ↓ leaves what's typed alone; it used to clear it.
	if m = press(m, tea.KeyDown); m.textarea.Value() != "my draft" {
		t.Errorf("↓ past the draft: %q", m.textarea.Value())
	}
}

// TestUpInsideATypedMultiLineDraft: in text you're writing, ↑ moves the cursor
// up a line first and reaches history only from the top row.
func TestUpInsideATypedMultiLineDraft(t *testing.T) {
	m := voiceModel("off")
	m.history = newInputHistory()
	m.textarea.Focus() // as it is when typing; an unfocused textarea ignores keys
	m.history.Push("older")
	m.textarea.SetValue("line one\nline two") // cursor at the end, on line two
	if m = press(m, tea.KeyUp); m.textarea.Value() != "line one\nline two" || m.textarea.Line() != 0 {
		t.Fatalf("↑ from line two: %q on line %d, want the cursor moved up", m.textarea.Value(), m.textarea.Line())
	}
	if m = press(m, tea.KeyUp); m.textarea.Value() != "older" {
		t.Fatalf("↑ from the top row: %q, want history", m.textarea.Value())
	}
	if m = press(m, tea.KeyDown); m.textarea.Value() != "line one\nline two" {
		t.Errorf("↓ back: %q, want the draft", m.textarea.Value())
	}
}
