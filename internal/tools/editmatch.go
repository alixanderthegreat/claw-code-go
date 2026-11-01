package tools

import "strings"

// looseMatch finds old_string in content when an exact match failed, comparing whole lines with
// their leading and trailing whitespace ignored: the model's commonest miss is the right text with
// spaces where the file has tabs, or a trailing space. It reports the byte span of the matching
// lines (the last line's newline excluded) and how many places match; the caller applies it only
// when exactly one does. old's leading and trailing blank lines don't count, and it needs at
// least one non-blank line. A line that differs in anything but surrounding whitespace never
// matches.
func looseMatch(content, old string) (start, end, n int) {
	want := trimBlankLines(strings.Split(old, "\n"))
	if len(want) == 0 {
		return 0, 0, 0
	}
	for i := range want {
		want[i] = strings.TrimSpace(want[i])
	}
	lines := strings.Split(content, "\n")
	offsets := make([]int, len(lines)+1) // offsets[i] is where line i starts
	for i, l := range lines {
		offsets[i+1] = offsets[i] + len(l) + 1
	}
	for i := 0; i+len(want) <= len(lines); i++ {
		ok := true
		for j, w := range want {
			if strings.TrimSpace(lines[i+j]) != w {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if n == 0 {
			start, end = offsets[i], offsets[i+len(want)]-1
		}
		n++
	}
	return start, end, n
}

// reindent rewrites new's indentation in the file's style, for a loose match: when the file's
// matched line is indented with tabs and the model's old_string with spaces, each leading run of
// spaces in new that is a whole number of the model's indent unit becomes that many tabs. Lines
// it can't map exactly are left as they are.
func reindent(new, fileLine, oldLine string) string {
	fileIndent := leading(fileLine)
	oldIndent := leading(oldLine)
	tabs := strings.Count(fileIndent, "\t")
	if tabs == 0 || oldIndent == "" || strings.Trim(oldIndent, " ") != "" || len(oldIndent)%tabs != 0 {
		return new
	}
	unit := len(oldIndent) / tabs // spaces the model used per tab
	ls := strings.Split(new, "\n")
	for i, l := range ls {
		sp := len(l) - len(strings.TrimLeft(l, " "))
		if sp > 0 && sp%unit == 0 {
			ls[i] = strings.Repeat("\t", sp/unit) + l[sp:]
		}
	}
	return strings.Join(ls, "\n")
}

func leading(s string) string { return s[:len(s)-len(strings.TrimLeft(s, " \t"))] }

func trimBlankLines(ls []string) []string {
	for len(ls) > 0 && strings.TrimSpace(ls[0]) == "" {
		ls = ls[1:]
	}
	for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// firstNonBlank is the first line of s with something on it.
func firstNonBlank(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			return l
		}
	}
	return ""
}
