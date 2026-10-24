package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

// markdown.go: the model's prose rendered as markdown with Glamour - headings, lists, tables,
// highlighted code - instead of raw **asterisks** and backticks. A text block streams in plain as
// it arrives and is rendered in place once it's complete (the next tool call, thinking, or the end
// of the reply): half-written markdown, an unclosed code fence above all, would reflow on every
// chunk. Tool lines and thinking keep their own styles.

// mdCache keeps one renderer per wrap width and theme: building one parses its whole style.
var mdCache struct {
	sync.Mutex
	width int
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
	wrap := max(width-4, 20) // Glamour's style indents two columns each side

	light := currentTheme == LightTheme
	mdCache.Lock()
	defer mdCache.Unlock()
	if mdCache.r == nil || mdCache.width != wrap || mdCache.light != light {
		style := "dark"
		if light {
			style = "light"
		}
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(wrap))
		if err != nil {
			return md
		}
		mdCache.r, mdCache.width, mdCache.light = r, wrap, light
	}
	out, err := mdCache.r.Render(md)
	if err != nil {
		return md
	}
	return "\n" + strings.Trim(out, "\n") + "\n"
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
