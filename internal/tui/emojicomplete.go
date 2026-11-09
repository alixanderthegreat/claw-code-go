package tui

import (
	"sort"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"claw-code-go/internal/signs"
)

// Emoji completion (#37), as in Slack and Matrix: after ":" and two letters, a row of matching
// emoji shows over the input and narrows as you type. ↑/↓ or Tab choose, Enter or Tab inserts,
// Esc dismisses.

// completeMax is how many matches the row offers.
const completeMax = 8

type emojiMatch struct {
	Emoji, Code string
}

type emojiComplete struct {
	query     string // what follows the ":" now, "" when nothing is being completed
	items     []emojiMatch
	at        int
	dismissed string // Esc'd at this query: stays hidden until the query changes
}

// sortedCodes is every GitHub shortcode, in order, for stable matches.
var sortedCodes = sync.OnceValue(func() []string {
	m := shortcodes()
	out := make([]string, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
})

func nameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '+' || r == '-'
}

// shortcodeQuery is the shortcode being typed at the end of before: the name after an opening
// ":" that starts a word (after a space, a line start or a bracket), at least two characters and
// not yet closed, and not inside backticks. "" when there's none.
func shortcodeQuery(before []rune) string {
	i := len(before) - 1
	for i >= 0 && nameRune(before[i]) {
		i--
	}
	if i < 0 || before[i] != ':' || len(before)-1-i < 2 {
		return ""
	}
	if i > 0 && !strings.ContainsRune(" \t\n([{", before[i-1]) {
		return ""
	}
	if strings.Count(string(before[:i]), "`")%2 == 1 {
		return ""
	}
	return string(before[i+1:])
}

// emojiMatches ranks shortcodes for a query: the language's signs first, then shortcodes that
// start with it, then ones that contain it. Each emoji once, at most n.
func emojiMatches(q string, n int) []emojiMatch {
	var out []emojiMatch
	seen := map[string]bool{}
	add := func(e, code string) bool {
		if !steadyWidth(e) {
			return false
		}
		// ☑ and ☑️ are one emoji: the second only asks for colour (U+FE0F).
		if k := strings.TrimSuffix(e, "\ufe0f"); !seen[k] {
			seen[k] = true
			out = append(out, emojiMatch{e, code})
		}
		return len(out) >= n
	}
	for _, s := range signs.Signs {
		if strings.HasPrefix(s.Code[1:], q) && add(s.Emoji, s.Code) {
			return out
		}
	}
	all := shortcodes()
	for _, prefix := range []bool{true, false} {
		for _, c := range sortedCodes() {
			name := c[1 : len(c)-1]
			if (prefix && strings.HasPrefix(name, q)) || (!prefix && !strings.HasPrefix(name, q) && strings.Contains(name, q)) {
				if add(all[c], c) {
					return out
				}
			}
		}
	}
	return out
}

// steadyWidth says an emoji is two columns by every measure, so the row is as wide on screen as
// it was laid out (#39). Joined sequences (👨‍🦯: three emoji and two zero-width joiners) are drawn
// as their parts by many terminals, and a text symbol asking for emoji form (⁉️) is one column to
// some measures and two to others; an emoji wider or narrower than planned shifts the row, and if
// it wraps, the frame is a line taller than the screen. Those still resolve by their exact
// shortcode; they're just not offered in the row.
func steadyWidth(e string) bool {
	return !strings.ContainsRune(e, '\u200d') && ansi.StringWidth(e) == 2 && runewidth.StringWidth(e) == 2
}

// completeMargin keeps the row this many columns short of the width, for a terminal that still
// draws a glyph wider than the measures say.
const completeMargin = 4

// refreshComplete recomputes the row from the text before the cursor, after every edit.
func (m *Model) refreshComplete() {
	q := shortcodeQuery(m.beforeCursor())
	if q == m.complete.query && q != "" {
		return
	}
	m.complete.query, m.complete.at = q, 0
	m.complete.items = nil
	if q != "" && q != m.complete.dismissed {
		m.complete.items = emojiMatches(q, completeMax)
	}
	if q != m.complete.dismissed {
		m.complete.dismissed = ""
	}
}

// completing says the row is showing, so the keys it uses go to it. It also checks the text still
// ends with the query, so a row left by an edit that didn't refresh it (Ctrl+U, history) is gone.
func (m Model) completing() bool {
	return len(m.complete.items) > 0 && shortcodeQuery(m.beforeCursor()) == m.complete.query
}

func (m Model) handleCompleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	n := len(m.complete.items)
	switch msg.Type {
	case tea.KeyUp:
		m.complete.at = (m.complete.at + n - 1) % n
	case tea.KeyDown:
		m.complete.at = (m.complete.at + 1) % n
	case tea.KeyEnter, tea.KeyTab:
		e := m.complete.items[m.complete.at].Emoji
		for range len([]rune(m.complete.query)) + 1 { // the name and its ":"
			m.textarea, _ = m.textarea.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
		m.textarea.InsertString(e)
		m.complete = emojiComplete{}
	case tea.KeyEsc:
		m.complete.dismissed, m.complete.items = m.complete.query, nil
	default:
		return m, nil, false
	}
	return m, nil, true
}

// renderComplete is the row: each match, the chosen one marked with its shortcode.
func (m Model) renderComplete() string {
	var b strings.Builder
	for i, it := range m.complete.items {
		if i == m.complete.at {
			b.WriteString("[" + it.Emoji + " " + it.Code + "] ")
		} else {
			b.WriteString(" " + it.Emoji + "  ")
		}
	}
	line := b.String() + " ↑↓ choose · Enter insert · Esc"
	if m.width > completeMargin {
		line = ansi.Truncate(line, m.width-completeMargin, "…")
	}
	return statusStyle.Render(line)
}
