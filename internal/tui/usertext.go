package tui

import "strings"

// userText is how the operator's message shows in the transcript: their label, then the message
// rendered as markdown like a reply, so a list, a table or a code block reads as it does in their
// editor. It's filtered by printable first (a paste can carry control characters), and each line
// break they typed is kept: in markdown a single newline joins lines into one paragraph.
func userText(text string, md *mdStore) string {
	return userLabelStyle.Render("You") + ": " + md.mark(keepLineBreaks(printable(text))) + "\n"
}

// keepLineBreaks makes each single newline a markdown hard break (two spaces at the line's end),
// outside fenced code blocks, where lines are kept anyway. Two spaces, not a backslash: at the end
// of a list item or a heading a backslash shows as itself, and two spaces are ignored. Blank
// lines, which already end a paragraph, and fence lines are left alone.
func keepLineBreaks(s string) string {
	lines := strings.Split(s, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		fence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
		if fence {
			inFence = !inFence
			continue
		}
		if inFence || i == len(lines)-1 || trimmed == "" || strings.TrimSpace(lines[i+1]) == "" {
			continue
		}
		next := strings.TrimSpace(lines[i+1])
		if strings.HasPrefix(next, "```") || strings.HasPrefix(next, "~~~") {
			continue
		}
		lines[i] = strings.TrimRight(line, " ") + "  "
	}
	return strings.Join(lines, "\n")
}
