package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// activity.go: what a running turn is doing, with the spinner, on its own row under the conversation
// - waiting on the model ("Thinking… 34s", which is also the wait while it reads a long prompt),
// running a tool, writing out a tool call. A screen that sits still for a minute looks the same as
// one that hung.

// setActivity says what the turn is doing now; timed shows how long.
func (m Model) setActivity(text string, timed bool) Model {
	return m.setActivityKeyed(text, text, timed)
}

// setActivityKeyed is setActivity for an activity whose text changes as it goes - a draft's growing
// size, a consult's token count: the clock restarts only when key does.
func (m Model) setActivityKeyed(key, text string, timed bool) Model {
	if key != m.activityKey {
		m.activityKey, m.activitySince = key, time.Now()
	}
	m.activity, m.activityTimed = text, timed
	return m
}

// clearActivity ends the activity, clock and all. Clearing only the text left the key, so the next
// turn's "Thinking…" matched the last one's and its clock ran on from the previous turn.
func (m Model) clearActivity() Model {
	m.activity, m.activityKey, m.activitySince, m.activityTimed = "", "", time.Time{}, false
	return m
}

// activityLine is what's going on while busy: "⠙ Thinking… 34s".
func (m Model) activityLine() string {
	if m.state != stateBusy || m.activity == "" {
		return ""
	}
	line := m.spinner.View() + " " + m.activity
	if m.activityTimed {
		if d := time.Since(m.activitySince); d >= time.Second {
			line += " " + formatElapsed(d)
		}
	}
	return line
}

func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// renderActivityRow is the row under the conversation, above the divider and the input, carrying
// the activity while a turn runs: "⠙ writing write_file 34s". It stays (blank) when idle, so the
// input doesn't move when a turn starts or ends.
func (m Model) renderActivityRow() string {
	act := m.activityLine()
	if act == "" {
		return ""
	}
	return toolRunningStyle.Render(ansi.Truncate(act, max(m.width, 0), "…"))
}
