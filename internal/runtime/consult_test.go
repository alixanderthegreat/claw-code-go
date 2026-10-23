package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"claw-code-go/internal/api"
)

// consultClient answers like the model in a mode: some thinking, then an answer, slowly enough that
// progress shows. It keeps the request it got.
type consultClient struct {
	got   api.CreateMessageRequest
	delay time.Duration
}

func (c *consultClient) StreamResponse(ctx context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.got = req
	ch := make(chan api.StreamEvent)
	go func() {
		defer close(ch)
		for _, ev := range []api.StreamEvent{
			{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "thinking_delta", Text: strings.Repeat("hmm ", 200)}},
			{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: "Data: one slot."}},
		} {
			select {
			case <-time.After(c.delay):
			case <-ctx.Done():
				return
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func consultLoop(client api.APIClient) *ConversationLoop {
	loop := NewConversationLoop(&Config{Model: jumbo, MaxTokens: 1024, ContextWindow: 65536}, client)
	loop.CtxAssembler = nil
	return loop
}

// TestConsultIsAFreshCallInTheMode: a consult sends only the tagged question and material - no
// history, no tools, no system prompt - and the answer comes back labelled as a view to weigh.
func TestConsultIsAFreshCallInTheMode(t *testing.T) {
	client := &consultClient{delay: 300 * time.Millisecond}
	loop := consultLoop(client)
	loop.Session.Messages = []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "secret history"}}}}
	events := make(chan TurnEvent, 16)

	r := loop.consult(context.Background(), events, map[string]any{
		"mode": "holodeck", "question": "One slot or two?", "material": "notes with a stray {REASON:pkd} tag"})
	if r.IsError || !strings.Contains(r.Content[0].Text, "Data: one slot.") || !strings.Contains(r.Content[0].Text, "not a source") {
		t.Fatalf("result = %+v", r)
	}
	req := client.got
	if req.System != "" || len(req.Tools) != 0 || len(req.Messages) != 1 {
		t.Fatalf("consult should be a fresh call, got system %q, %d tools, %d messages", req.System, len(req.Tools), len(req.Messages))
	}
	prompt := req.Messages[0].Content[0].Text
	if !strings.HasPrefix(prompt, "{REASON:holodeck} One slot or two?") || strings.Contains(prompt, "{REASON:pkd}") || strings.Contains(prompt, "secret history") {
		t.Errorf("prompt = %q", prompt)
	}
	close(events)
	var progress []string
	for ev := range events {
		if ev.Type == TurnEventToolProgress {
			progress = append(progress, ev.ToolInput)
		}
	}
	if len(progress) == 0 || !strings.HasPrefix(progress[0], "holodeck - thinking") {
		t.Errorf("progress = %q, want it to say it's thinking", progress)
	}
}

// TestConsultRefusals: a mode that isn't one, instruct form, material that can't fit.
func TestConsultRefusals(t *testing.T) {
	client := &consultClient{}
	loop := consultLoop(client)
	events := make(chan TurnEvent, 16)
	if r := loop.consult(context.Background(), events, map[string]any{"mode": "xcoder", "question": "q"}); !r.IsError {
		t.Errorf("unknown mode should be refused")
	}
	loop.consult(context.Background(), events, map[string]any{"mode": "deeptree", "question": "q", "instruct": true})
	if !strings.HasPrefix(client.got.Messages[0].Content[0].Text, "{REASON:ideeptree} ") {
		t.Errorf("instruct should send the i-form tag, sent %q", client.got.Messages[0].Content[0].Text)
	}
	huge := strings.Repeat("x", 65536*charsPerToken)
	if r := loop.consult(context.Background(), events, map[string]any{"mode": "deeptree", "question": "q", "material": huge}); !r.IsError || !strings.Contains(r.Content[0].Text, "Send less material") {
		t.Errorf("oversized material should be refused before sending: %+v", r.Content[0].Text[:80])
	}
	if !strings.Contains(ConsultTool().Description, "deeptree (tree of thought") {
		t.Errorf("the tool should list its modes")
	}
	if !hasTool(loop.allTools(), "consult") || hasTool(NewConversationLoop(&Config{Model: "gpt-4o"}, client).allTools(), "consult") {
		t.Errorf("consult should be offered on mode models only")
	}
}

// TestConsultStopsOnCancel: Esc during a consult stops it, not waits it out.
func TestConsultStopsOnCancel(t *testing.T) {
	loop := consultLoop(&consultClient{delay: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	done := make(chan api.ContentBlock)
	go func() {
		done <- loop.consult(ctx, make(chan TurnEvent, 4), map[string]any{"mode": "spoon", "question": "q"})
	}()
	select {
	case r := <-done:
		if !r.IsError || !strings.Contains(r.Content[0].Text, "stopped") {
			t.Errorf("cancelled consult = %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("consult didn't stop when its turn was cancelled")
	}
}

func hasTool(tools []api.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}
