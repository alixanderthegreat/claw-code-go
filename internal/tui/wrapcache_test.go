package tui

import (
	"math/rand"
	"strings"
	"testing"
)

// The cache must be invisible: at every step of a stream - chunks splitting words, lines, styled
// runs and blank lines - its output is exactly what wrapping the whole transcript gives.
func TestWrapCacheMatchesFullWrap(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	pieces := []string{"word ", "longerword ", "a-hyphenated-name ", "\n", "\n\n",
		thinkingStyle.Render("styled thinking text that runs on "), "x", "  indented ",
		strings.Repeat("unbroken", 30), "\t"}
	for _, width := range []int{20, 47, 80} {
		c := &wrapCache{}
		var content string
		for step := 0; step < 600; step++ {
			content += pieces[r.Intn(len(pieces))]
			if got, want := c.wrap(content, width), wrapAtSpaces(content, width); got != want {
				t.Fatalf("width %d, step %d: cached wrap differs\ncontent %q\ngot  %q\nwant %q", width, step, content, got, want)
			}
		}
	}
}

// A resize, or a transcript rewritten rather than extended (a compaction, Esc), starts again.
func TestWrapCacheStartsAgain(t *testing.T) {
	c := &wrapCache{}
	a := "first line that is long enough to wrap\nsecond line\nthird"
	c.wrap(a, 20)
	if got, want := c.wrap(a, 33), wrapAtSpaces(a, 33); got != want {
		t.Fatalf("after a resize: got %q want %q", got, want)
	}
	b := "something else entirely\nand more"
	if got, want := c.wrap(b, 33), wrapAtSpaces(b, 33); got != want {
		t.Fatalf("after a rewrite: got %q want %q", got, want)
	}
	var nilCache *wrapCache
	if got, want := nilCache.wrap(a, 20), wrapAtSpaces(a, 20); got != want {
		t.Fatalf("nil cache: got %q want %q", got, want)
	}
}

// What a chunk costs at the end of a long transcript: with the cache, only the tail is wrapped.
func BenchmarkRefreshWrapLongTranscript(b *testing.B) {
	transcript := strings.Repeat(strings.Repeat("word ", 400)+"\n", 500) // ~1 MB
	b.Run("full", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = wrapAtSpaces(transcript+"next chunk", 100)
		}
	})
	b.Run("cached", func(b *testing.B) {
		c := &wrapCache{}
		c.wrap(transcript, 100)
		for i := 0; i < b.N; i++ {
			_ = c.wrap(transcript+"next chunk", 100)
		}
	})
}
