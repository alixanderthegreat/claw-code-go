package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestAskUserShowsTheWholeQuestion: the box wraps the whole question rather than cutting it at 200
// characters, and the question is in the chat too (greatproject #357).
func TestAskUserShowsTheWholeQuestion(t *testing.T) {
	m := voiceModel("off")
	m.width, m.height, m.state = 80, 40, stateBusy
	q := "The image shows a classic org pyramid. " + strings.Repeat("Section C-5.1 is about the loops. ", 8) + "LAST WORD"

	mm, _ := m.Update(streamAskUserMsg{question: q, reply: make(chan string, 1)})
	m = mm.(Model)
	box := ansi.Strip(m.viewAskUser())
	if !strings.Contains(box, "LAST") || !strings.Contains(box, "WORD") {
		t.Errorf("the box should hold the whole question:\n%s", box)
	}
	if !strings.Contains(m.viewBuf, "LAST WORD") {
		t.Error("the question should be in the chat too")
	}

	m.height = 20 // too short: cut, and say where the rest is
	if box := ansi.Strip(m.viewAskUser()); !strings.Contains(box, "in the chat above") {
		t.Errorf("a cut question should say where the rest is:\n%s", box)
	}
}
