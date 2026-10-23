package runtime

import (
	"context"
	"testing"
)

// TestDraftHint: what a call is about shows as soon as its arguments name it.
func TestDraftHint(t *testing.T) {
	for in, want := range map[string]string{
		``:     "",
		`{"pa`: "",
		`{"path": "internal/x.go", "content": "pa`: "path: internal/x.go",
		`{"command":"go test ./...`:                "command: go test ./...",
		`{"content":"lots","file_path":"a\"b.go"`:  `file_path: a\"b.go`,
	} {
		if got := draftHint(in); got != want {
			t.Errorf("draftHint(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestToolDraftComesBeforeTheToolRuns: while the model writes a call, the caller hears about it -
// the TUI used to show nothing until the call was complete and running.
func TestToolDraftComesBeforeTheToolRuns(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "true"}}
	loop := newScriptedLoop(client, nil)
	events := make(chan TurnEvent, 64)
	go func() {
		loop.SendMessageStreaming(context.Background(), "go", events)
		close(events)
	}()
	var order []TurnEventType
	var draft TurnEvent
	for ev := range events {
		if ev.Type == TurnEventToolDraft || ev.Type == TurnEventToolStart {
			order = append(order, ev.Type)
			if ev.Type == TurnEventToolDraft {
				draft = ev
			}
		}
	}
	if len(order) < 2 || order[0] != TurnEventToolDraft || order[len(order)-1] != TurnEventToolStart {
		t.Fatalf("tool events in order %v, want drafting before the run", order)
	}
	if draft.ToolName != "bash" {
		t.Errorf("draft named %q, want bash", draft.ToolName)
	}
}
