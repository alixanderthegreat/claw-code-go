package runtime

import (
	"context"
	"strings"
	"testing"

	"claw-code-go/internal/permissions"
)

// TestUnknownToolFailsWithoutAsking: in einstein Gordi called a "consult" tool, and the permission
// box asked the operator to allow a tool that doesn't exist (greatproject #432). It fails at once,
// and the error says how to run a command on PATH.
func TestUnknownToolFailsWithoutAsking(t *testing.T) {
	client := &scriptedClient{tool: "consult", input: map[string]any{"MODE": "joker", "question": "q"}}
	loop := newScriptedLoop(client, nil)
	loop.PermManager = permissions.NewManager(permissions.ModeDefault, nil)
	events := make(chan TurnEvent, 64)
	go func() {
		defer close(events)
		loop.SendMessageStreaming(context.Background(), "go", events)
	}()
	for ev := range events {
		if ev.Type == TurnEventPermissionAsk {
			t.Fatalf("asked permission for %s, a tool that doesn't exist", ev.ToolName)
		}
	}
	if len(client.requests) < 2 {
		t.Fatalf("the model never heard back: %d requests", len(client.requests))
	}
	var got string
	for _, m := range client.requests[1].Messages {
		for _, b := range m.Content {
			if b.Type == "tool_result" {
				for _, c := range b.Content {
					got += c.Text
				}
			}
		}
	}
	if !strings.Contains(got, "unknown tool: consult") || !strings.Contains(got, `bash {"command": "consult ...`) {
		t.Errorf("tool result = %q, want the unknown-tool error and the bash hint", got)
	}
}
