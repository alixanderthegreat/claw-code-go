package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

// TestStreamReportsCachedTokens: kronk says how much of the prompt its cache served in the final
// usage chunk (prompt_tokens_details.cached_tokens), and the TUI shows it - so it has to survive
// the trip to message_start. The request also names this client to simple-bot's gateway.
func TestStreamReportsCachedTokens(t *testing.T) {
	var client string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client = r.Header.Get("X-Client")
		w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n" +
			"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":16974,\"completion_tokens\":489,\"prompt_tokens_details\":{\"cached_tokens\":16801}}}\n\n" +
			"data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Model: "m", HTTPClient: srv.Client()}
	ch, err := c.StreamResponse(context.Background(), api.CreateMessageRequest{
		Messages: []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hello"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var start *api.StreamEvent
	for ev := range ch {
		if ev.Type == api.EventMessageStart {
			start = &ev
		}
	}
	if start == nil || start.InputTokens != 16974 || start.CachedTokens != 16801 {
		t.Fatalf("message_start = %+v, want 16974 input tokens with 16801 cached", start)
	}
	if client != "claw-code-go" {
		t.Errorf("X-Client = %q, want claw-code-go", client)
	}
}

// TestImagesBecomeImageURLParts: an image in a user message makes its content an array of a
// text part and image_url parts; an image a tool returned follows the tool message as a user
// message of its own, since a tool message is text only.
func TestImagesBecomeImageURLParts(t *testing.T) {
	img := api.ContentBlock{Type: "image", Source: &api.ImageSource{Type: "base64", MediaType: "image/png", Data: "QUJD"}}
	msgs := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "what's in shot.png?"}, img}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "tool_use", ID: "c1", Name: "read_file", Input: map[string]any{"path": "logo.png"}}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "tool_result", ToolUseID: "c1", Content: []api.ContentBlock{{Type: "text", Text: "logo.png is an image"}, img}}}},
	}
	raw, err := json.Marshal(convertMessages("", msgs))
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	json.Unmarshal(raw, &got)
	if len(got) != 4 {
		t.Fatalf("%d messages: %s", len(got), raw)
	}
	parts, ok := got[0]["content"].([]any)
	if !ok || len(parts) != 2 || parts[0].(map[string]any)["text"] != "what's in shot.png?" ||
		parts[1].(map[string]any)["image_url"].(map[string]any)["url"] != "data:image/png;base64,QUJD" {
		t.Errorf("the user message: %s", raw)
	}
	if got[2]["role"] != "tool" || got[2]["content"] != "logo.png is an image" {
		t.Errorf("the tool message: %v", got[2])
	}
	if p, ok := got[3]["content"].([]any); got[3]["role"] != "user" || !ok || len(p) != 2 {
		t.Errorf("the tool's image should follow as a user message: %v", got[3])
	}
	if !strings.Contains(string(raw), `"role":"assistant"`) || strings.Contains(string(raw), `"Parts"`) {
		t.Errorf("shape: %s", raw)
	}
}
