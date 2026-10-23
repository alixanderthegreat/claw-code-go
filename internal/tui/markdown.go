package tui

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
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
		m.streamBuf = m.streamBuf[:m.mdStart] + renderMarkdown(m.mdText, m.width)
	}
	m.mdText = ""
	return m
}

// terminalWidth is the terminal's width before Bubble Tea has reported one, 0 if unknown.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return w
}
