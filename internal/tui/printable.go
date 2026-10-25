package tui

import (
	"strings"
	"unicode/utf8"
)

// printable is s made safe to draw: invalid UTF-8 becomes "�", and control characters - escape
// sequences, carriage returns, bells - are dropped, keeping newlines and tabs. A tool's raw
// output used to reach the screen as it was: binary from a grep over an MP3 carried escape
// codes that ran the following lines together.
func printable(s string) string {
	clean := func(s string) bool {
		for _, r := range s {
			if r == utf8.RuneError || (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r < 0xa0) {
				return false
			}
		}
		return true
	}
	if utf8.ValidString(s) && clean(s) {
		return s
	}
	s = strings.ToValidUTF8(s, "�")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r < 0xa0) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
