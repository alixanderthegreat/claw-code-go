package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"claw-code-go/internal/runtime"
)

func TestShortModel(t *testing.T) {
	for in, want := range map[string]string{
		"DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-plusIQ-TOOLS-NEO-MAX-MTP-Q6_K": "9B plusIQ MTP",
		"DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-NEO-MAX-Q6_K":                  "9B NEO-MAX",
		"gpt-4o": "gpt-4o",
	} {
		if got := shortModel(in); got != want {
			t.Errorf("shortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPanelTakesTheHeaderHintsAndStatus: on a wide terminal the header, the key hints and the
// status bar are in the panel and not across the bottom, and the chat gets their three lines;
// narrow, they stay where they were (greatproject #437).
func TestPanelTakesTheHeaderHintsAndStatus(t *testing.T) {
	cfg := &runtime.Config{Model: "DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-plusIQ-TOOLS-NEO-MAX-MTP-Q6_K", ProviderName: "openai"}
	view := func(width int) (Model, string) {
		m := NewModel(cfg, runtime.NewConversationLoop(cfg, &recordingClient{}))
		mm, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 50})
		m = mm.(Model)
		return m, ansi.Strip(m.View())
	}
	wide, w := view(130)
	if strings.Contains(w, "Enter=send") || strings.Contains(w, "Claw Code v") {
		t.Errorf("wide: the hint or header row is still across the chat")
	}
	for _, want := range []string{"9B plusIQ MTP", "openai · v", "session …", "Ctrl+O details"} {
		if !strings.Contains(w, want) {
			t.Errorf("wide: panel lacks %q", want)
		}
	}
	for _, line := range strings.Split(w, "\n") {
		if ansi.StringWidth(line) > 130 {
			t.Errorf("wide: a line of %d columns", ansi.StringWidth(line))
		}
	}
	narrow, n := view(90)
	if !strings.Contains(n, "Enter=send") || !strings.Contains(n, "Claw Code v") {
		t.Errorf("narrow: the hint and header rows are gone")
	}
	if wide.viewport.Height != narrow.viewport.Height+3 {
		t.Errorf("viewport %d lines wide, %d narrow: want 3 more beside the panel", wide.viewport.Height, narrow.viewport.Height)
	}
	if got := strings.Count(w, "\n"); got > 50 {
		t.Errorf("wide view is %d lines on a 50-line terminal", got+1)
	}
}
