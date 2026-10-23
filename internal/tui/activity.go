package tui

import (
	"fmt"
	"time"
)

// activity.go: what a running turn is doing, beside the spinner in the status bar - waiting on the
// model ("Thinking… 34s", which is also the wait while it reads a long prompt), running a tool,
// writing out a tool call. A screen that sits still for a minute looks the same as one that hung.

// setActivity says what the turn is doing now; timed shows how long. The clock restarts only when
// the activity changes, so repeated updates of the same one keep counting.
func (m Model) setActivity(text string, timed bool) Model {
	if text != m.activity {
		m.activity, m.activitySince = text, time.Now()
	}
	m.activityTimed = timed
	return m
}

// activityLine is the status bar's lead while busy: "⠙ Thinking… 34s".
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
