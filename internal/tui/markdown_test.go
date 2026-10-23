package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestRenderMarkdown: the reply's markdown comes out formatted - no raw ** or code fences - wrapped
// to the terminal, and on its own line below the label. (Glamour's styles keep a styled "## " in
// front of second-level headings on purpose, so that one stays.)
func TestRenderMarkdown(t *testing.T) {
	md := "## Done\n\nI fixed **two** bugs in `compact.go`:\n\n- the summary request\n- retention\n\n```go\nfunc f() {}\n```\n\n" +
		strings.Repeat("word ", 60)
	out := ansi.Strip(renderMarkdown(md, 60))
	for _, want := range []string{"Done", "two", "compact.go", "the summary request", "func f() {}"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output lost %q:\n%s", want, out)
		}
	}
	for _, raw := range []string{"**two**", "`compact.go`", "```"} {
		if strings.Contains(out, raw) {
			t.Errorf("rendered output still has raw %q:\n%s", raw, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Errorf("line %d wide, over the 60-column terminal: %q", ansi.StringWidth(line), line)
		}
	}
	if !strings.HasPrefix(out, "\n") {
		t.Errorf("rendered block should start below the label")
	}
}

// TestStreamedTextRenderedInPlace: text streams in plain, and the moment its block ends - here, a
// tool call - it's replaced by its rendering; what came before it, and the tool line, are untouched.
func TestStreamedTextRenderedInPlace(t *testing.T) {
	m := voiceModel("off")
	m.width = 80
	m.streamBuf = "BEFORE"
	for _, chunk := range []string{"Here is **bo", "ld** text."} {
		mm, _ := m.Update(streamDeltaMsg{text: chunk})
		m = mm.(Model)
	}
	if !strings.Contains(m.streamBuf, "**bold**") {
		t.Fatalf("while streaming, the text should be as it arrived: %q", m.streamBuf)
	}
	mm, _ := m.Update(streamToolMsg{name: "bash", input: "ls"})
	m = mm.(Model)
	plain := ansi.Strip(m.streamBuf)
	if !strings.HasPrefix(plain, "BEFORE") || strings.Contains(plain, "**") || !strings.Contains(plain, "bold") || !strings.Contains(plain, "◆ bash: ls") {
		t.Errorf("after the tool call, streamBuf = %q", plain)
	}
	if m.mdOpen {
		t.Errorf("the block should be closed after the tool call")
	}
}
