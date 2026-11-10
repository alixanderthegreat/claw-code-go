package runtime

import (
	"context"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

// lineClient answers every request with text, stopped as stop/finish say.
type lineClient struct {
	text, stop, finish string
	last               api.CreateMessageRequest
}

func (c *lineClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.last = req
	ch := make(chan api.StreamEvent, 4)
	defer close(ch)
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: c.text}}
	ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: c.stop, Usage: api.UsageDelta{FinishReason: c.finish}}
	ch <- api.StreamEvent{Type: api.EventMessageStop}
	return ch, nil
}

// A suggestion is shown whole or not at all (#60): one cut off by the token limit (either
// vocabulary) or longer than suggestMaxChars gives none, never half a sentence.
func TestSuggestionNeverShownCut(t *testing.T) {
	msgs := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "build it"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "Built. Merge it?"}}},
	}
	long := strings.Repeat("word ", 50)
	for _, c := range []struct {
		name string
		cl   lineClient
		want string
	}{
		{"short", lineClient{text: "merge it", stop: "end_turn", finish: "stop"}, "merge it"},
		{"cut, anthropic", lineClient{text: "So the token cap is a hard", stop: "max_tokens"}, ""},
		{"cut, openai", lineClient{text: "So the token cap is a hard", stop: "end_turn", finish: "length"}, ""},
		{"too long", lineClient{text: long, stop: "end_turn", finish: "stop"}, ""},
	} {
		cl := c.cl
		loop := NewConversationLoop(&Config{Model: "m", MaxTokens: 100}, &cl)
		got, err := loop.SuggestNext(context.Background(), msgs)
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

// The context says whose words are whose, and the ask wants the operator's voice, briefly.
func TestSuggestionContextNamesTheVoices(t *testing.T) {
	cl := lineClient{text: "ok", stop: "end_turn"}
	loop := NewConversationLoop(&Config{Model: "m", MaxTokens: 100}, &cl)
	loop.SuggestNext(context.Background(), []api.Message{ //nolint:errcheck
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "build it"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "Built."}}},
	})
	text := cl.last.Messages[0].Content[0].Text
	for _, want := range []string{"the assistant's words, not the operator's", "at most 20 words", "in the operator's voice"} {
		if !strings.Contains(text, want) {
			t.Errorf("the request lacks %q", want)
		}
	}
}
