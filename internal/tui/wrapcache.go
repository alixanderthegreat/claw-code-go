package tui

import "strings"

// wrapcache.go: the transcript, wrapped once. refreshViewport runs on every streamed chunk, and it
// wrapped the whole transcript each time - about 1 ms per 20 KB, so in a long session (1 MB, 40 ms
// a chunk) the screen couldn't keep up and replies stuttered in. The transcript only grows at its
// end, so the lines already complete are wrapped once and kept, and a chunk re-wraps only what
// came after the last of them. Anything else - a resize, a compaction, Esc trimming the turn -
// starts again from the whole transcript.
type wrapCache struct {
	width   int
	raw     string // the content wrapped so far, up to and including its last "\n"
	wrapped string // wrapAtSpaces of raw without that final "\n"
}

// wrap returns wrapAtSpaces(content, width), reusing what's unchanged since the last call. A nil
// cache wraps everything, every time.
func (c *wrapCache) wrap(content string, width int) string {
	if c == nil {
		return wrapAtSpaces(content, width)
	}
	if width != c.width || !strings.HasPrefix(content, c.raw) {
		c.width, c.raw, c.wrapped = width, "", ""
	}
	rest := content[len(c.raw):]
	if j := strings.LastIndexByte(rest, '\n'); j >= 0 {
		done := wrapAtSpaces(rest[:j], width)
		if c.raw == "" {
			c.wrapped = done
		} else {
			c.wrapped += "\n" + done
		}
		c.raw += rest[:j+1]
		rest = rest[j+1:]
	}
	if c.raw == "" {
		return wrapAtSpaces(rest, width)
	}
	return c.wrapped + "\n" + wrapAtSpaces(rest, width)
}
