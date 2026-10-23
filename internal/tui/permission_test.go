package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/ansi"
)

// TestPermissionShowsItAll: what's being allowed is readable to the end - it used to stop at 60
// characters, which cut a bash command, or a question, short - and the screen still fits.
func TestPermissionShowsItAll(t *testing.T) {
	m := voiceModel("off")
	m.width, m.height = 80, 24
	m.viewport = viewport.New(80, 16)
	m.permToolName = "bash"
	m.permToolInput = "cd /home/user/dev/play/context/simpleproject/cmd/simple-calendar && git push -u origin main && echo THE-END"
	view := m.viewPermissionOverlay()
	plain := strings.Join(strings.Fields(ansi.Strip(view)), " ")
	if !strings.Contains(plain, "THE-END") {
		t.Errorf("the command was cut short:\n%s", ansi.Strip(view))
	}
	if h := strings.Count(view, "\n") + 1; h > m.height {
		t.Errorf("overlay is %d lines on a %d-line screen", h, m.height)
	}
}
