package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func shown(t *testing.T, text string) string {
	t.Helper()
	md := newMDStore()
	return ansi.Strip(md.expand(userText(text, md), 80))
}

func TestUserTextIsMarkdown(t *testing.T) {
	got := shown(t, "make it **bold** and `code`")
	if strings.Contains(got, "**") || !strings.Contains(got, "bold") || !strings.Contains(got, "You") {
		t.Errorf("bold not rendered: %q", got)
	}
	list := shown(t, "- one\n- two")
	if !strings.Contains(list, "• one") || !strings.Contains(list, "• two") || strings.Contains(list, `\`) {
		t.Errorf("list not rendered: %q", list)
	}
}

// The operator's own line breaks stay: a message typed over three lines shows on three.
func TestUserTextKeepsLines(t *testing.T) {
	got := shown(t, "07:00\nchildren hear more than they say\n07:01")
	var lines []string
	for _, l := range strings.Split(got, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 4 || lines[1] != "07:00" || lines[3] != "07:01" { // "You:", then the three
		t.Errorf("lines joined: %q", lines)
	}
}

func TestKeepLineBreaks(t *testing.T) {
	in := "a\nb\n\nc\n```\nx\ny\n```\nd"
	want := "a  \nb\n\nc\n```\nx\ny\n```\nd"
	if got := keepLineBreaks(in); got != want {
		t.Errorf("keepLineBreaks:\n got %q\nwant %q", got, want)
	}
}

func TestUserTextFiltersControls(t *testing.T) {
	if got := shown(t, "ok\x1b[2Jcleared?\x07"); strings.Contains(got, "\x1b[2J") || strings.Contains(got, "\x07") {
		t.Errorf("control characters reached the screen: %q", got)
	}
}
