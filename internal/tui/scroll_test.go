package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

// streamingModel is a model mid-reply with more lines than its 10-line viewport shows.
func streamingModel(t *testing.T) Model {
	t.Helper()
	m := voiceModel("off")
	m.width, m.height, m.state = 80, 30, stateBusy
	m.viewport = viewport.New(80, 10)
	for i := 0; i < 40; i++ {
		mm, _ := m.Update(streamDeltaMsg{text: fmt.Sprintf("line %d of the reply\n", i)})
		m = mm.(Model)
	}
	if !m.viewport.AtBottom() {
		t.Fatalf("a fresh reply should follow the output")
	}
	return m
}

// TestScrolledUpStaysPut (#8): scrolled up to read what Gordi wrote, the next chunk doesn't jump
// the view back down. The new text is in the content all the same.
func TestScrolledUpStaysPut(t *testing.T) {
	m := streamingModel(t)
	m.viewport.SetYOffset(5)
	mm, _ := m.Update(streamDeltaMsg{text: "a new line while you read\n"})
	m = mm.(Model)
	if m.viewport.YOffset != 5 {
		t.Errorf("view moved to offset %d while scrolled up; want it left at 5", m.viewport.YOffset)
	}
	m.viewport.GotoBottom()
	if !strings.Contains(m.viewport.View(), "a new line while you read") {
		t.Errorf("the new text isn't in the content")
	}
}

// TestAtBottomFollows: at the bottom, more output keeps the view at the bottom.
func TestAtBottomFollows(t *testing.T) {
	m := streamingModel(t)
	mm, _ := m.Update(streamDeltaMsg{text: "one more line\n"})
	m = mm.(Model)
	if !m.viewport.AtBottom() || !strings.Contains(m.viewport.View(), "one more line") {
		t.Errorf("at the bottom, the view should follow the new line")
	}
}

// TestTurnEndLeavesTheReaderAlone: the turn ending doesn't drag a scrolled-up reader down.
func TestTurnEndLeavesTheReaderAlone(t *testing.T) {
	m := streamingModel(t)
	m.viewport.SetYOffset(3)
	mm, _ := m.Update(streamDoneMsg{})
	m = mm.(Model)
	if m.viewport.YOffset != 3 {
		t.Errorf("the turn's end moved the view to %d; want it left at 3", m.viewport.YOffset)
	}
}
