package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TestAvatarFollowsGordi: each state has its own face, and the idle one blinks now and then
// (greatproject #433).
func TestAvatarFollowsGordi(t *testing.T) {
	faces := map[string]avatarState{}
	for _, s := range []avatarState{avatarIdle, avatarThinking, avatarTool, avatarAsking} {
		faces[gridString(avatarGrid(s, 1))] = s
	}
	if len(faces) != 4 {
		t.Errorf("idle, thinking, working and asking share faces: %d distinct of 4", len(faces))
	}
	if gridString(avatarGrid(avatarIdle, avatarBlinkGap)) == gridString(avatarGrid(avatarIdle, 1)) {
		t.Errorf("the idle gopher never blinks")
	}
	if gridString(avatarGrid(avatarSpeaking, 1)) == gridString(avatarGrid(avatarSpeaking, 2)) {
		t.Errorf("the mouth doesn't move while speaking")
	}
	if gridString(avatarGrid(avatarThinking, 0)) == gridString(avatarGrid(avatarThinking, 2)) {
		t.Errorf("the eyes don't drift while thinking")
	}
}

// TestAvatarFitsBesideTheChat: on a wide terminal the avatar shows and no line runs past the edge;
// on a narrow one it hides, gives the chat every column and arms no tick.
func TestAvatarFitsBesideTheChat(t *testing.T) {
	m := voiceModel("off")
	m.viewBuf = "You: " + strings.Repeat("what's at the port ", 30) + "\n"
	mm, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mm.(Model)
	if cmd == nil || !m.avatarTicking {
		t.Errorf("a wide terminal didn't arm the avatar's tick")
	}
	view := m.withAvatar(m.viewport.View()) // the test model has no loop for the header
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 120 {
			t.Errorf("a line of %d columns on a 120-column terminal", w)
		}
	}
	if m.viewport.Width != 120-avatarCols {
		t.Errorf("viewport width %d, want %d beside the avatar", m.viewport.Width, 120-avatarCols)
	}

	n := voiceModel("off")
	nn, cmd := n.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	n = nn.(Model)
	if cmd != nil || n.avatarTicking || n.viewport.Width != 80 {
		t.Errorf("narrow terminal: tick %v, ticking %v, width %d; want none, false, 80", cmd != nil, n.avatarTicking, n.viewport.Width)
	}
	if n.withAvatar("x") != "x" {
		t.Errorf("the avatar showed on a narrow terminal")
	}
}

func gridString(g [][]byte) string {
	var rows []string
	for _, r := range g {
		rows = append(rows, string(r))
	}
	return strings.Join(rows, "\n")
}
