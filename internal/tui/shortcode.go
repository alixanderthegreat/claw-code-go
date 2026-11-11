package tui

import (
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyokomi/emoji/v2"

	"claw-code-go/internal/signs"
)

// Emoji shortcodes resolve as they're typed (#33): most terminals have no emoji picker, so
// `:checkered_flag:` is the way to type 🏁. The set is GitHub's and Slack's, after the eleven
// signs of the starting language (`:stuck:` -> 🧱).
var shortcodes = sync.OnceValue(emoji.CodeMap)

// shortcodeAt looks at the text before the cursor. If it ends with a known shortcode that just
// closed (":name:"), it returns the emoji and how many runes the shortcode takes; otherwise "", 0.
// Unknown names stay as typed, and so does anything inside backticks, where colons are code.
func shortcodeAt(before []rune) (string, int) {
	n := len(before)
	if n < 3 || before[n-1] != ':' {
		return "", 0
	}
	open := -1
	for i := n - 2; i >= 0; i-- {
		r := before[i]
		if r == ':' {
			open = i
			break
		}
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '+' || r == '-') {
			return "", 0
		}
	}
	if open < 0 || open == n-2 {
		return "", 0
	}
	// An odd number of backticks before it: inside inline code or a fence.
	if strings.Count(string(before[:open]), "`")%2 == 1 {
		return "", 0
	}
	code := string(before[open:])
	e, ok := signs.SignCode(code) // the language's own signs first (#35)
	if !ok {
		e, ok = shortcodes()[code]
	}
	// An emoji of unsteady width stays as its shortcode (#47): drawn in the input, it would
	// garble the screen.
	if !ok || unsteadyEmoji(e) {
		return "", 0
	}
	return e, n - open
}

// beforeCursor is the input's text up to the cursor: the rows above it and the current row to
// the cursor's column.
func (m *Model) beforeCursor() []rune {
	lines := strings.Split(m.textarea.Value(), "\n")
	row := m.textarea.Line()
	if row >= len(lines) {
		return nil
	}
	li := m.textarea.LineInfo()
	cur := []rune(lines[row])
	col := min(li.StartColumn+li.ColumnOffset, len(cur))
	out := []rune(strings.Join(lines[:row], "\n"))
	if row > 0 {
		out = append(out, '\n')
	}
	return append(out, cur[:col]...)
}

// resolveShortcode replaces a shortcode the cursor just closed with its emoji, through the
// textarea's own keys so its cursor and wrapping stay right.
func (m *Model) resolveShortcode() {
	e, n := shortcodeAt(m.beforeCursor())
	if n == 0 {
		return
	}
	for range n {
		m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m.textarea.InsertString(e)
}
