package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// activity.go: what a running turn is doing, with the spinner, on the divider just above the input
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

// renderDivider is the rule between the conversation and the input, carrying the activity while a
// turn runs: "──⠙ writing write_file · 34s ──────".
func (m Model) renderDivider() string {
	act := m.activityLine()
	if act == "" {
		return dividerStyle.Render(strings.Repeat("─", max(m.width, 0)))
	}
	const lead, leadWidth = "──", 2
	act = ansi.Truncate(act, max(m.width-leadWidth-2, 0), "…")
	rest := max(m.width-leadWidth-ansi.StringWidth(act)-2, 0)
	return dividerStyle.Render(lead) + " " + toolRunningStyle.Render(act) + " " + dividerStyle.Render(strings.Repeat("─", rest))
}
