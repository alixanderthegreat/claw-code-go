package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
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
