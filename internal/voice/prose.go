package voice

import (
	"regexp"
	"strings"
)

var (
	reFence      = regexp.MustCompile("(?s)```.*?(```|$)")
	reInlineCode = regexp.MustCompile("`([^`\n]*)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	reURL        = regexp.MustCompile(`https?://\S+`)
	reEmphasis   = regexp.MustCompile(`(\*\*|__|\*|~~)`)
	reListMarker = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s+`)
	reHeading    = regexp.MustCompile(`^\s*#{1,6}\s+`)
	reSpaces     = regexp.MustCompile(`[ \t]+`)
)

// Prose is the part of a markdown reply worth saying aloud: code blocks, tables and URLs dropped,
// inline code kept only when short enough to say, formatting marks removed.
func Prose(md string) string {
	md = reFence.ReplaceAllString(md, "\n")
	md = reInlineCode.ReplaceAllStringFunc(md, func(s string) string {
		inner := reInlineCode.FindStringSubmatch(s)[1]
		if len(inner) > 40 {
			return ""
		}
		return inner
	})
	md = reLink.ReplaceAllString(md, "$1")
	md = reURL.ReplaceAllString(md, "")

	var out []string
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "|") || strings.HasPrefix(t, ">") && strings.Trim(t, "> ") == "" {
			continue // tables, empty quotes
		}
		t = strings.TrimLeft(t, "> ")
		t = reHeading.ReplaceAllString(t, "")
		t = reListMarker.ReplaceAllString(t, "")
		t = reEmphasis.ReplaceAllString(t, "")
		t = reSpaces.ReplaceAllString(t, " ")
		if strings.Trim(t, "-=_* ") == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "") // keep paragraph breaks, drop rules and blank runs
			}
			continue
		}
		out = append(out, t)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Chunks splits text into pieces to synthesize one after another, at sentence ends. The first is
// short so speech starts quickly; the rest are up to maxLen bytes, and a sentence longer than that
// is cut at a space.
func Chunks(text string, firstLen, maxLen int) []string {
	var chunks []string
	var cur strings.Builder
	limit := firstLen
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			chunks = append(chunks, s)
			limit = maxLen
		}
		cur.Reset()
	}
	for _, s := range sentences(text) {
		for len(s) > maxLen {
			cut := strings.LastIndex(s[:maxLen], " ")
			if cut <= 0 {
				cut = maxLen
			}
			flush()
			cur.WriteString(s[:cut])
			flush()
			s = strings.TrimSpace(s[cut:])
		}
		if cur.Len() > 0 && cur.Len()+1+len(s) > limit {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(s)
	}
	flush()
	return chunks
}

// sentences splits at . ! ? followed by whitespace, and at paragraph breaks.
func sentences(text string) []string {
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.Join(strings.Fields(para), " ")
		start := 0
		for i := 0; i < len(para); i++ {
			if (para[i] == '.' || para[i] == '!' || para[i] == '?') && (i+1 == len(para) || para[i+1] == ' ') {
				if s := strings.TrimSpace(para[start : i+1]); s != "" {
					out = append(out, s)
				}
				start = i + 1
			}
		}
		if s := strings.TrimSpace(para[start:]); s != "" {
			out = append(out, s)
		}
	}
	return out
}
