package runtime

import (
	"context"
	"regexp"
	"time"
)

// tool_draft.go: word that a tool call is being written. A write_file's arguments are the whole
// file, so the model can spend minutes writing one before anything runs - and the stream showed
// nothing until it did. While a call's arguments arrive, TurnEventToolDraft says which tool, what
// it's for once that's in (a path, a command), and how much has come.

// draftInterval spaces the updates for one call: often enough to see it move, not one per token.
const draftInterval = 250 * time.Millisecond

type toolDraftNotifier struct {
	last time.Time
}

// send reports the progress of the call to name whose arguments so far are args - always when it
// starts, otherwise at most every draftInterval. It returns false only if ctx ended while sending.
func (n *toolDraftNotifier) send(ctx context.Context, events chan<- TurnEvent, name, args string, start bool) bool {
	now := time.Now()
	if !start && now.Sub(n.last) < draftInterval {
		return true
	}
	n.last = now
	select {
	case events <- TurnEvent{Type: TurnEventToolDraft, ToolName: name, ToolInput: draftHint(args), DraftBytes: len(args)}:
		return true
	case <-ctx.Done():
		return false
	}
}

// reDraftField finds the argument that says what a call is about, in arguments still arriving.
var reDraftField = regexp.MustCompile(`"(path|file_path|command|pattern|url|query)"\s*:\s*"((?:[^"\\]|\\.){1,120})`)

// draftHint is "path: internal/x.go" once the arguments name it, or "".
func draftHint(partial string) string {
	head := partial
	if len(head) > 2048 { // the field that names it comes first; don't rescan a growing file
		head = head[:2048]
	}
	m := reDraftField.FindStringSubmatch(head)
	if m == nil {
		return ""
	}
	return m[1] + ": " + m[2]
}
