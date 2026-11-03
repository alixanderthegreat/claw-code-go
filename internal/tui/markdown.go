package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// markdown.go: the model's prose rendered as markdown with Glamour - headings, lists, tables,
// highlighted code - instead of raw **asterisks** and backticks. A text block streams in plain as
// it arrives and is rendered in place once it's complete (the next tool call, thinking, or the end
// of the reply): half-written markdown, an unclosed code fence above all, would reflow on every
// chunk. Tool lines and thinking keep their own styles.

// mdCache keeps one renderer per theme: building one parses its whole style.
var mdCache struct {
	sync.Mutex
	light bool
	r     *glamour.TermRenderer
}

// renderMarkdown renders md for a terminal width columns wide, or returns it unchanged if Glamour
// can't. The result starts on its own line, below the speaker's label.
func renderMarkdown(md string, width int) string {
	if strings.TrimSpace(md) == "" {
		return md
	}
	if width <= 0 {
		width = 80
	}
	wrap := max(width-2, 20) // Glamour's style indents two columns; keep two clear on the right

	light := currentTheme == LightTheme
	mdCache.Lock()
	defer mdCache.Unlock()
	if mdCache.r == nil || mdCache.light != light {
		style := "dark"
		if light {
			style = "light"
		}
		// No wrapping in Glamour (#6): it wraps a paragraph, pads it, then wraps it again in every
		// block with a margin, at a width the padded line no longer fits - "or" alone on a line.
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(0))
		if err != nil {
			return md
		}
		mdCache.r, mdCache.light = r, light
	}
	out, err := mdCache.r.Render(md)
	if err != nil {
		return md
	}
	return "\n" + hangWrap(strings.Trim(out, "\n"), wrap) + "\n"
}

// mdLead is a rendered line's lead: its indent, then a bullet, a list number or a quote bar.
var mdLead = regexp.MustCompile(`^( *)((?:•|\d+\.|│) )?`)

// hangWrap wraps each line of Glamour's unwrapped output to width once, keeping its styles, with
// the continuation lines hung under the line's text: below a bullet's or a number's words, and a
// quote's bar repeated.
func hangWrap(s string, width int) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if ansi.StringWidth(line) <= width {
			b.WriteString(line)
			continue
		}
		m := mdLead.FindStringSubmatch(ansi.Strip(line))
		lead := ansi.StringWidth(m[0])
		if lead > width/2 {
			b.WriteString(wrapAtSpaces(line, width))
			continue
		}
		hang := m[1] + strings.Repeat(" ", ansi.StringWidth(m[2]))
		if strings.HasPrefix(m[2], "│") {
			hang = m[1] + m[2]
		}
		body := strings.Split(wrapAtSpaces(ansi.TruncateLeft(line, lead, ""), width-lead), "\n")
		b.WriteString(ansi.Truncate(line, lead, "") + body[0])
		for _, l := range body[1:] {
			b.WriteString("\n" + hang + l)
		}
	}
	return b.String()
}

// closeMarkdown renders the text block that was streaming, in place, now that it's complete.
func (m Model) closeMarkdown() Model {
	if !m.mdOpen {
		return m
	}
	m.mdOpen = false
	if m.mdStart <= len(m.streamBuf) {
		m.streamBuf = m.streamBuf[:m.mdStart] + m.md.mark(m.mdText)
	}
	m.mdText = ""
	return m
}

// mdStore holds each rendered block's markdown, so the transcript carries a placeholder and the
// block is rendered at the width the screen has now. Rendered once and baked in, a reply kept the
// width it was written at: after a resize, every line too wide for the window was wrapped again,
// a few words dropping to the left edge mid-paragraph.
type mdStore struct {
	mu     sync.Mutex
	raw    []string
	width  int
	render []string // raw[i] rendered at width; "" until asked for
}

func newMDStore() *mdStore { return &mdStore{} }

var reMDMark = regexp.MustCompile("\x00md([0-9]+)\x00")

// mark stores md and returns the placeholder that stands for it in the transcript.
func (s *mdStore) mark(md string) string {
	if s == nil {
		return renderMarkdown(md, 80)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append(s.raw, md)
	s.render = append(s.render, "")
	return fmt.Sprintf("\x00md%d\x00", len(s.raw)-1)
}

// expand replaces content's placeholders with their blocks rendered at width, rendering each
// once per width.
func (s *mdStore) expand(content string, width int) string {
	if s == nil || !strings.Contains(content, "\x00md") {
		return content
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if width != s.width {
		s.width = width
		for i := range s.render {
			s.render[i] = ""
		}
	}
	return reMDMark.ReplaceAllStringFunc(content, func(mark string) string {
		i, err := strconv.Atoi(reMDMark.FindStringSubmatch(mark)[1])
		if err != nil || i >= len(s.raw) {
			return ""
		}
		if s.render[i] == "" {
			s.render[i] = renderMarkdown(s.raw[i], width)
		}
		return s.render[i]
	})
}
