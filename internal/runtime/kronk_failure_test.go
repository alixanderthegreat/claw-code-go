package runtime

import (
	"context"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

// cutOffClient answers with kronk's cut-off sentence `cuts` times, then "done".
type cutOffClient struct {
	cuts     int
	requests []api.CreateMessageRequest
}

func (c *cutOffClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.requests = append(c.requests, req)
	text := "done"
	if len(c.requests) <= c.cuts {
		text = kronkLengthTerminated
	}
	ch := make(chan api.StreamEvent, 8)
	ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: text}}
	ch <- api.StreamEvent{Type: api.EventContentBlockStop}
	ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
	ch <- api.StreamEvent{Type: api.EventMessageStop}
	close(ch)
	return ch, nil
}

func runCutOff(t *testing.T, cuts int) (*cutOffClient, []TurnEvent) {
	t.Helper()
	client := &cutOffClient{cuts: cuts}
	loop := NewConversationLoop(&Config{Model: "test-model", MaxTokens: 8096, ContextWindow: 40960}, client)
	loop.CtxAssembler = nil
	events := make(chan TurnEvent, 64)
	go func() {
		loop.SendMessageStreaming(context.Background(), "write the HTML report", events)
		close(events)
	}()
	var got []TurnEvent
	for ev := range events {
		got = append(got, ev)
	}
	return client, got
}

// TestCutOffCallIsRetried: kronk's "Response truncated before completion." is
// for the model, not the answer - it used to end Gordi's turn (seen
// 2026-10-24, writing an HTML report). The model hears why and goes on.
func TestCutOffCallIsRetried(t *testing.T) {
	client, events := runCutOff(t, 1)
	if len(client.requests) != 2 {
		t.Fatalf("%d requests, want the cut-off one and a retry", len(client.requests))
	}
	msgs := client.requests[1].Messages
	note := msgs[len(msgs)-1].Content[0].Text
	if !strings.Contains(note, "8096-token output limit") || !strings.Contains(note, "in parts") {
		t.Errorf("the retry's note was %q", note)
	}
	for _, ev := range events {
		if ev.Type == TurnEventError {
			t.Errorf("an error after one cut-off: %v", ev.Err)
		}
	}
}

// TestCutOffsGiveUp: a model that keeps cutting off is stopped with an error,
// not retried forever.
func TestCutOffsGiveUp(t *testing.T) {
	client, events := runCutOff(t, 100)
	if len(client.requests) != maxKronkRetries+1 {
		t.Errorf("%d requests, want %d", len(client.requests), maxKronkRetries+1)
	}
	var gaveUp error
	for _, ev := range events {
		if ev.Type == TurnEventError {
			gaveUp = ev.Err
		}
	}
	if gaveUp == nil || !strings.Contains(gaveUp.Error(), "truncated") {
		t.Errorf("it ended with error %v, want one naming the cut-off", gaveUp)
	}
}
