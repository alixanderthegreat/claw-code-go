package runtime

import (
	"regexp"
	"strings"

	"claw-code-go/internal/api"
)

// A reply that ends on a promise - "Next: I'll pick #165" - and makes no tool call ends the turn
// there: the step is announced and never taken, and the operator is left reading narration with no
// answer (greatproject #435). The loop nudges once to take the step or end with a report.

// promiseTail matches a last sentence that announces work instead of doing it.
var promiseTail = regexp.MustCompile(`(?i)(^|[.!:\n]\s*)(next:|next,|i'll |i will |i'm going to |let me |now i'll |now let me )`)

// notAPromise are endings that start like one but hand the turn back: an offer or a request.
var notAPromise = regexp.MustCompile(`(?i)let me know|i'll wait|i will wait|if you (want|like|say)|once you|when you|your call|\?\s*$`)

const promiseNudge = "[claw-code-go] Your reply ends by saying what you'll do next, and the turn stops there, so it " +
	"never happens. Do that step now with a tool call, or end the turn with a report: what was done, " +
	"with proof (the path, commit or link), and what waits on the operator."

// endsOnPromise reports whether an assistant message's text ends by announcing a next step. It
// reads the last sentence only: a promise earlier in a reply that went on to report isn't one.
func endsOnPromise(m api.Message) bool {
	var text strings.Builder
	for _, b := range m.Content {
		if b.Type == "tool_use" {
			return false
		}
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	t := strings.TrimSpace(text.String())
	if t == "" || notAPromise.MatchString(lastSentences(t, 1)) {
		return false
	}
	return promiseTail.MatchString(lastSentences(t, 1))
}

// lastSentences is s's last n sentences, roughly: lines and ". ", "! ", "? " end one.
func lastSentences(s string, n int) string {
	ends := regexp.MustCompile(`[.!?]\s+|\n+`).FindAllStringIndex(s, -1)
	if len(ends) < n {
		return s
	}
	return s[ends[len(ends)-n][1]:]
}
