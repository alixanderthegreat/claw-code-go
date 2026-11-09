package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"claw-code-go/internal/signs"
)

// The sign picker (#35): Ctrl+G opens a one-line palette of the twelve signs over the input, since
// most terminals have no emoji picker. ←/→ move, a key from signKeys picks directly, Enter inserts
// the one selected, Esc or Ctrl+G closes.
type signPicker struct {
	open bool
	at   int
}

// signKeys picks a sign directly: 1-9, 0, -, = for the twelve, in order.
const signKeys = "1234567890-="

func (m Model) handleSignKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(signs.Signs)
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlG:
		m.signs.open = false
	case tea.KeyLeft:
		m.signs.at = (m.signs.at + n - 1) % n
	case tea.KeyRight, tea.KeyTab:
		m.signs.at = (m.signs.at + 1) % n
	case tea.KeyEnter:
		m = m.insertSign(m.signs.at)
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			if i := strings.IndexRune(signKeys, msg.Runes[0]); i >= 0 && i < n {
				m = m.insertSign(i)
			}
		}
	}
	return m, nil
}

func (m Model) insertSign(i int) Model {
	m.textarea.InsertString(signs.Signs[i].Emoji)
	m.signs.open = false
	return m
}

// renderSignPicker is the palette line: each sign with its key, the selected one marked, and what
// it means.
func (m Model) renderSignPicker() string {
	var b strings.Builder
	for i, s := range signs.Signs {
		cell := fmt.Sprintf("%c %s", signKeys[i], s.Emoji)
		if i == m.signs.at {
			cell = "[" + cell + "]"
		} else {
			cell = " " + cell + " "
		}
		b.WriteString(cell)
	}
	sel := signs.Signs[m.signs.at]
	line := b.String() + "  " + sel.Code + " " + sel.Means
	if m.width > 0 {
		line = ansi.Truncate(line, m.width, "…")
	}
	return statusStyle.Render(line)
}
