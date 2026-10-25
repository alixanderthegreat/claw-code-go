package runtime

import (
	"context"
	"strings"
	"testing"
)

func runTurn(loop *ConversationLoop, text string) {
	events := make(chan TurnEvent, 64)
	go func() { loop.SendMessageStreaming(context.Background(), text, events); close(events) }()
	for range events {
	}
}

// TestInterjectionReachesTheNextStep: a message sent while a turn runs goes to the model with the
// next round's tool results - it used to wait for the whole turn to end.
func TestInterjectionReachesTheNextStep(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "true"}}
	loop := newScriptedLoop(client, nil)
	loop.Interject("stop - use b.txt, not a.txt")
	runTurn(loop, "fix a.txt")
	if len(client.requests) < 2 {
		t.Fatalf("%d requests", len(client.requests))
	}
	msgs := client.requests[1].Messages
	last := msgs[len(msgs)-1]
	var sawResult, sawNote bool
	for _, b := range last.Content {
		sawResult = sawResult || b.Type == "tool_result"
		sawNote = sawNote || (b.Type == "text" && strings.Contains(b.Text, "stop - use b.txt") && strings.Contains(b.Text, "while you were working"))
	}
	if !sawResult || !sawNote {
		t.Errorf("the second request's last message: %+v", last.Content)
	}
	if left := loop.TakeInterjections(); len(left) != 0 {
		t.Errorf("still pending after it was read: %q", left)
	}
}

// TestInterjectionWithoutAStepWaits: a turn that answers without calling a tool never reads it,
// so it stays pending for the TUI to send next.
func TestInterjectionWithoutAStepWaits(t *testing.T) {
	loop := NewConversationLoop(&Config{Model: "test-model", MaxTokens: 1024, ContextWindow: 40960}, &cutOffClient{})
	loop.CtxAssembler = nil
	loop.Interject("and one more thing")
	runTurn(loop, "hi")
	if left := loop.TakeInterjections(); len(left) != 1 || left[0] != "and one more thing" {
		t.Errorf("pending after a turn with no step: %q", left)
	}
}
