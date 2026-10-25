package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestAltEnterIsANewline: Alt+Enter, Ctrl+N and Ctrl+J break the line and send
// nothing; plain Enter sends; Ctrl+U clears.
func TestAltEnterIsANewline(t *testing.T) {
	m := voiceModel("off")
	m.history = newInputHistory()
	m.textarea.Focus()
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEnter, Alt: true}, {Type: tea.KeyCtrlN}, {Type: tea.KeyCtrlJ}} {
		m.textarea.SetValue("para one")
		mm, _ := m.Update(k)
		m = mm.(Model)
		if m.textarea.Value() != "para one\n" {
			t.Errorf("%v: %q, want a newline added", k, m.textarea.Value())
		}
		if isSendKey(k) {
			t.Errorf("%v sends", k)
		}
	}
	if m.textarea.SetValue("scrap this"); press(m, tea.KeyCtrlU).textarea.Value() != "" {
		t.Error("Ctrl+U left the input")
	}
	if !isSendKey(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Error("plain Enter doesn't send")
	}
}

// TestCtrlArrowsMoveByWord: Ctrl+←/→ skip words and Ctrl+Backspace deletes
// one, as in most editors; only Alt did before.
func TestCtrlArrowsMoveByWord(t *testing.T) {
	m := voiceModel("off")
	m.textarea = newInput()
	m.history = newInputHistory()
	m.textarea.Focus()
	m.textarea.SetValue("one two three")
	m = press(m, tea.KeyCtrlLeft)
	if c := m.textarea.LineInfo().CharOffset; c != 8 {
		t.Errorf("Ctrl+← from the end: cursor at %d, want 8, the start of three", c)
	}
	m = press(m, tea.KeyCtrlLeft, tea.KeyCtrlRight)
	if c := m.textarea.LineInfo().CharOffset; c != 7 {
		t.Errorf("Ctrl+← then Ctrl+→: cursor at %d, want 7, the end of two", c)
	}
	m = press(m, tea.KeyCtrlH)
	if v := m.textarea.Value(); v != "one  three" {
		t.Errorf("Ctrl+Backspace: %q, want two deleted", v)
	}
}

// TestHistorySurvivesARestart: entries pushed in one run - newlines and all -
// are there for ↑ in the next.
func TestHistorySurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input_history.jsonl")
	first := loadInputHistory(path)
	first.Push("one")
	first.Push("two\nlines")
	first.Push("two\nlines") // a repeat isn't kept twice

	again := loadInputHistory(path)
	if len(again.entries) != 2 || again.entries[1] != "two\nlines" {
		t.Fatalf("after a restart: %q", again.entries)
	}
	if got := again.Prev(""); got != "two\nlines" {
		t.Errorf("↑ after a restart: %q", got)
	}
}

// TestHistoryFileStaysBounded: a file past twice the cap is cut back to the
// newest entries when it's loaded.
func TestHistoryFileStaysBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input_history.jsonl")
	h := loadInputHistory(path)
	for i := 0; i < 2*maxHistoryEntries+10; i++ {
		h.Push(fmt.Sprintf("entry %d", i))
	}
	again := loadInputHistory(path)
	if len(again.entries) != maxHistoryEntries || again.entries[len(again.entries)-1] != fmt.Sprintf("entry %d", 2*maxHistoryEntries+9) {
		t.Fatalf("%d entries, last %q", len(again.entries), again.entries[len(again.entries)-1])
	}
	raw, _ := os.ReadFile(path)
	if n := strings.Count(string(raw), "\n"); n != maxHistoryEntries {
		t.Errorf("file has %d lines after the rewrite, want %d", n, maxHistoryEntries)
	}
}
