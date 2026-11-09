package tui

import (
	"sort"
	"strings"
	"sync"

	"github.com/kyokomi/emoji/v2"
	"github.com/rivo/uniseg"
)

// An emoji built from several code points (👨‍⚖️, 👋🏻, flags, keycaps), or a symbol asking for
// emoji form (⚖️), is drawn wider than the two columns every width measure gives it by many
// terminals, VTE among them: the line wraps, the frame is a line taller than laid out, and each
// frame lands below the last (#47). The screen shows such an emoji as its shortcode instead; the
// text itself, as sent and kept, is unchanged.

// revCodes maps each emoji to one shortcode for it: the shortest, then the first in order.
var revCodes = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	for e, codes := range emoji.RevCodeMap() {
		c := append([]string(nil), codes...)
		sort.Slice(c, func(i, j int) bool {
			if len(c[i]) != len(c[j]) {
				return len(c[i]) < len(c[j])
			}
			return c[i] < c[j]
		})
		out[e] = c[0]
	}
	return out
})

// unsteadyEmoji says a grapheme is an emoji whose drawn width depends on the terminal: one with
// a joiner, a variation selector, a keycap, a skin tone, flag letters or tag characters.
func unsteadyEmoji(g string) bool {
	if steadyWidth(g) {
		return false
	}
	for _, r := range g {
		switch {
		case r == 0x200d, r == 0xfe0f, r == 0x20e3,
			r >= 0x1f3fb && r <= 0x1f3ff, // skin tones
			r >= 0x1f1e6 && r <= 0x1f1ff, // flag letters
			r >= 0xe0020 && r <= 0xe007f: // tags
			return true
		}
	}
	return false
}

// shortcodeFor is how the screen shows an unsteady emoji: its shortcode, else the steady emoji
// it starts with (👋 for 👋🏻), else "�".
func shortcodeFor(g string) string {
	codes := revCodes()
	for _, k := range []string{g, strings.ReplaceAll(g, "️", ""), g + "️"} {
		if c, ok := codes[k]; ok {
			return c
		}
	}
	if first := string([]rune(g)[:1]); steadyWidth(first) {
		return first
	}
	return "�"
}

// steadyText is s with each unsteady emoji drawn as its shortcode. Text without one is returned
// as it is.
func steadyText(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= 0x2000 }) {
		return s
	}
	var b strings.Builder
	changed := false
	gr := uniseg.NewGraphemes(s)
	for gr.Next() {
		g := gr.Str()
		if unsteadyEmoji(g) {
			b.WriteString(shortcodeFor(g))
			changed = true
			continue
		}
		b.WriteString(g)
	}
	if !changed {
		return s
	}
	return b.String()
}
