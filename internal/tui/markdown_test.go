package tui

import (
	"regexp"
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
	plain := ansi.Strip(m.md.expand(m.streamBuf, 80))
	if !strings.HasPrefix(plain, "BEFORE") || strings.Contains(plain, "**") || !strings.Contains(plain, "bold") || !strings.Contains(plain, "◆ bash: ls") {
		t.Errorf("after the tool call, streamBuf = %q", plain)
	}
	if m.mdOpen {
		t.Errorf("the block should be closed after the tool call")
	}
}

// TestRenderedRepliesRewrapOnResize: a reply is laid out at the width the screen has now, not the
// one it was written at - narrowed, the old line breaks left words stranded at the left edge.
func TestRenderedRepliesRewrapOnResize(t *testing.T) {
	m := voiceModel("off")
	para := strings.Repeat("System 2 is the lazy controller and it only engages when necessary. ", 6)
	m.viewBuf = "Gordi: " + m.md.mark(para) + "\n"
	for _, w := range []int{168, 100} {
		got := ansi.Strip(m.md.expand(m.viewBuf, w))
		for _, line := range strings.Split(got, "\n") {
			if ansi.StringWidth(line) > w {
				t.Errorf("width %d: a line of %d columns", w, ansi.StringWidth(line))
			}
		}
		want := ansi.Strip(renderMarkdown(para, w))
		if !strings.Contains(got, strings.TrimSpace(want)) {
			t.Errorf("width %d: not laid out for this width", w)
		}
	}
}

// TestMarkdownNoOrphanedWords (#6): Glamour wrapped a paragraph and then wrapped it again, a little
// narrower, so a line near the limit lost its last word to a line of its own ("or", then "are you
// going to try it yourself?" at 87 columns). Wrapped once, no line of a paragraph is a lone word
// with more of the paragraph after it, at any width, and none is wider than the screen.
func TestMarkdownNoOrphanedWords(t *testing.T) {
	md := `Simple task - change the output to "Hello, World!". Want me to make the edit, or are you going to try it yourself?`
	for w := 40; w <= 160; w++ {
		lines := strings.Split(ansi.Strip(renderMarkdown(md, w)), "\n")
		for i, l := range lines {
			if ansi.StringWidth(l) > w {
				t.Errorf("width %d: a line of %d columns: %q", w, ansi.StringWidth(l), l)
			}
			s := strings.TrimSpace(l)
			if s != "" && !strings.Contains(s, " ") && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				t.Errorf("width %d: %q alone on line %d, with %q after it", w, s, i, strings.TrimSpace(lines[i+1]))
			}
		}
	}
}

// TestMarkdownHangingIndent: a wrapped list item continues under its words, not under its bullet
// or number, and a wrapped quote keeps its bar on every line.
func TestMarkdownHangingIndent(t *testing.T) {
	long := "item that runs long enough to need wrapping at least once at this width, surely"
	md := "- " + long + "\n\n1. numbered " + long + "\n\n> a quote " + long + "\n"
	lines := strings.Split(ansi.Strip(renderMarkdown(md, 50)), "\n")
	after := func(prefix string) string {
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimLeft(l, " "), prefix) && i+1 < len(lines) {
				return lines[i+1]
			}
		}
		t.Fatalf("no line starting %q in:\n%s", prefix, strings.Join(lines, "\n"))
		return ""
	}
	for _, c := range []struct{ first, want string }{
		{"• item", "    "},       // two of margin, then under the text after "• "
		{"1. numbered", "     "}, // under the text after "1. "
		{"│ a quote", "  │ "},
	} {
		next := after(c.first)
		if !strings.HasPrefix(next, c.want) || strings.TrimSpace(next) == "" || next[len(c.want)] == ' ' {
			t.Errorf("after %q the next line is %q, want it to start %q then text", c.first, next, c.want)
		}
	}
}

// TestMarkdownKeepsStylesAcrossWrap: wrapping happens after Glamour, on styled text, so a bold
// phrase that wraps is still bold on its second line, and nothing of the text is lost.
func TestMarkdownKeepsStylesAcrossWrap(t *testing.T) {
	md := "Plain words first, then **a bold phrase long enough that it has to wrap onto the next line here**, then plain."
	out := renderMarkdown(md, 50)
	plain := strings.Join(strings.Fields(ansi.Strip(out)), " ")
	if !strings.Contains(plain, "a bold phrase long enough that it has to wrap onto the next line here") {
		t.Errorf("text lost in the wrap: %q", plain)
	}
	var wrappedBold bool
	for _, l := range strings.Split(out, "\n") {
		// bold is SGR 1, wherever it sits among the parameters: "\x1b[1;38;5;252m", "\x1b[38;5;252;1m"
		if strings.Contains(ansi.Strip(l), "next line") && regexp.MustCompile(`\x1b\[(?:[0-9;]*;)?1(?:;[0-9;]*)?m`).MatchString(l) {
			wrappedBold = true
		}
	}
	if !wrappedBold {
		t.Errorf("the bold phrase's second line isn't bold:\n%q", out)
	}
}
