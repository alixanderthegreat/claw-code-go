package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/x/ansi"
)

// TestActivityThroughATurn: the row under the conversation says what the turn is doing at every stage - a tool call
// being written, the tool running, and the wait on the model after it, with how long that's been -
// and the wait shows even after output has arrived, which is when the screen used to sit still.
func TestActivityThroughATurn(t *testing.T) {
	m := voiceModel("off")
	m.spinner = spinner.New()
	m.state, m.hasStreamContent = stateBusy, true

	mm, _ := m.Update(streamToolDraftMsg{key: "draft write_file", text: draftLine("write_file", "path: internal/x.go", 3300)})
	m = mm.(Model)
	if got := ansi.Strip(m.activityLine()); !strings.HasSuffix(got, "writing write_file - path: internal/x.go · 3.2 KB") {
		t.Errorf("drafting: %q", got)
	}
	// A draft's size grows, but its clock keeps running - kronk sends no size at all, only time.
	m.activitySince = time.Now().Add(-20 * time.Second)
	mm, _ = m.Update(streamToolDraftMsg{key: "draft write_file", text: draftLine("write_file", "", 0)})
	m = mm.(Model)
	if got := ansi.Strip(m.activityLine()); !strings.HasSuffix(got, "writing write_file 20s") {
		t.Errorf("draft clock should keep counting: %q", got)
	}
	m.width = 20
	if row := ansi.Strip(m.renderActivityRow()); ansi.StringWidth(row) > 20 || !strings.Contains(row, "writing") {
		t.Errorf("activity row = %q, want it within the 20-column width", row)
	}

	mm, _ = m.Update(streamToolMsg{name: "write_file", input: "internal/x.go"})
	m = mm.(Model)
	if got := ansi.Strip(m.activityLine()); !strings.HasSuffix(got, "Running write_file…") {
		t.Errorf("running: %q", got)
	}

	mm, _ = m.Update(streamToolDoneMsg{name: "write_file", result: "ok"})
	m = mm.(Model)
	m.activitySince = time.Now().Add(-34 * time.Second) // the model reading a long prompt
	if got := ansi.Strip(m.activityLine()); !strings.HasSuffix(got, "Thinking… 34s") {
		t.Errorf("waiting after the tool: %q", got)
	}

	mm, _ = m.Update(streamDoneMsg{})
	m = mm.(Model)
	if m.activityLine() != "" {
		t.Errorf("activity should clear when the turn ends")
	}
}

// TestSpinnerTicksAllTurn: the spinner keeps going for the whole turn, output or not.
func TestSpinnerTicksAllTurn(t *testing.T) {
	m := voiceModel("off")
	m.spinner = spinner.New()
	m.state, m.hasStreamContent = stateBusy, true
	if _, cmd := m.Update(m.spinner.Tick()); cmd == nil {
		t.Errorf("a busy turn with output stopped the spinner")
	}
}

// TestActivityClockPerTurn: each turn's "Thinking…" starts from zero, not where the last one ended.
func TestActivityClockPerTurn(t *testing.T) {
	m := voiceModel("off")
	m.state = stateBusy
	m = m.setActivity("Thinking…", true)
	m.activitySince = time.Now().Add(-90 * time.Second) // a long first turn
	mm, _ := m.Update(streamDoneMsg{})
	m = mm.(Model)
	m.state = stateBusy
	m = m.setActivity("Thinking…", true) // the next turn starts
	if d := time.Since(m.activitySince); d > time.Second {
		t.Errorf("the next turn's clock started at %s, want 0", d.Round(time.Second))
	}
}
