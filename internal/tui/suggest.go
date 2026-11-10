package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/api"
	"claw-code-go/internal/suggestions"
)

// Next-prompt suggestions (#32): after each reply, the operator's likely next message shows as
// ghost text in the empty input, and Tab takes it into the box to edit or send. It never sends
// itself. Each one is logged with what the operator then sent, so whether they help is measured.

const inputPlaceholder = "Type a message or /help..."

type nextSuggestion struct {
	gen   int    // which turn's suggestion is wanted: a late answer for an older turn is dropped
	text  string // shown now, "" when none
	taken bool   // Tab put it in the box
}

type suggestionMsg struct {
	gen  int
	text string
}

// suggestCmd asks for a suggestion after the turn that just ended, off the UI's goroutine.
func (m Model) suggestCmd() tea.Cmd {
	if m.cfg == nil || !m.cfg.SuggestNext || m.loop == nil || m.loop.Client == nil {
		return nil
	}
	msgs := append([]api.Message(nil), m.loop.Session.Messages...)
	gen, loop := m.suggest.gen, m.loop
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		text, err := loop.SuggestNext(ctx, msgs)
		if err != nil {
			return nil
		}
		return suggestionMsg{gen: gen, text: text}
	}
}

// showSuggestion puts a suggestion in the empty input as its placeholder, if it's still for this
// turn and the operator hasn't started typing.
func (m Model) showSuggestion(msg suggestionMsg) Model {
	if msg.gen != m.suggest.gen || m.state != stateInput || m.textarea.Value() != "" || !suggestionAllowed(msg.text) {
		return m
	}
	m.suggest.text = msg.text
	m.textarea.Placeholder = msg.text + "   ⇥ Tab"
	return m
}

// takeSuggestion is Tab on an empty input with a suggestion showing: it goes into the box.
func (m Model) takeSuggestion() Model {
	m.textarea.SetValue(m.suggest.text)
	m.textarea.CursorEnd()
	m.suggest.taken = true
	m.textarea.Placeholder = inputPlaceholder
	return m
}

// endSuggestion logs what became of the suggestion when the operator sends something (or a new
// turn starts without one), and clears it.
func (m Model) endSuggestion(sent string) Model {
	if m.suggest.text != "" {
		logSuggestion(m.suggest.text, sent, m.suggest.taken)
	}
	m.suggest = nextSuggestion{gen: m.suggest.gen + 1}
	m.textarea.Placeholder = inputPlaceholder
	return m
}

// suggestionAllowed says whether a suggestion may be offered: any that isn't empty, unless it looks
// outward and the outward gate holds those.
//
// The trial, the operator's call (2026-11-09): #32 proposed holding back an approval of an outward
// action (a push, a deploy, a merge, a message sent), so that one key can't approve it. Each logged
// suggestion says whether it looked outward, and the log settles it (#50): once there are enough
// outward ones and too few were accepted, they're held (suggestions.Held). Inward suggestions are
// always offered. Permissions and the fences are unchanged either way: a suggestion is only text in
// the box.
func suggestionAllowed(s string) bool {
	return s != "" && (!looksOutward(s) || !outwardHeld())
}

// outwardHeld says whether the log holds outward suggestions back. It reads the log once a process:
// the rates move slowly, and a turn shouldn't read a file. When it holds, it says so once, as a line
// in the log itself (the screen is the TUI's), which the reader skips as a note, not a suggestion.
var outwardHeld = outwardGate(suggestionsLog)

func outwardGate(path func() string) func() bool {
	return sync.OnceValue(func() bool {
		p := path()
		held, why := suggestions.HeldAt(p)
		if held {
			appendLog(p, map[string]any{"time": time.Now().Format(time.RFC3339), "event": why})
		}
		return held
	})
}

// outwardWords mark a suggestion that approves something leaving this machine, for the trial's
// log (looksOutward). Whole words, lower case.
var outwardWords = []string{"push", "deploy", "merge", "send", "publish", "post", "release", "ship"}

// looksOutward says whether a suggestion contains one of outwardWords as a word of its own
// ("push it", "merge em"; not "pushover").
func looksOutward(s string) bool {
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		for _, o := range outwardWords {
			if w == o {
				return true
			}
		}
	}
	return false
}

// suggestionsLog is where each suggestion and its outcome go, on this machine only.
func suggestionsLog() string { return suggestions.Path() }

// logSuggestion appends one outcome: accepted (sent as suggested), edited (taken, then changed),
// or ignored (the operator typed their own).
func logSuggestion(suggested, sent string, taken bool) {
	outcome := "ignored"
	switch {
	case taken && sent == suggested:
		outcome = "accepted"
	case taken:
		outcome = "edited"
	}
	appendLog(suggestionsLog(), map[string]any{
		"time": time.Now().Format(time.RFC3339), "suggested": suggested, "sent": sent, "outcome": outcome,
		"outward": looksOutward(suggested),
	})
}

// appendLog adds one JSON line to the log at p, private to the operator.
func appendLog(p string, rec map[string]any) {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	line, _ := json.Marshal(rec)
	f.Write(append(line, '\n')) //nolint:errcheck
}
