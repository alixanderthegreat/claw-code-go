package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

// NoThinking reaches the server as chat_template_kwargs, and only when asked for: an ordinary
// request is sent as before (#32).
func TestNoThinking(t *testing.T) {
	c := &Client{Model: "m"}
	msgs := []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}}}
	plain, err := c.buildRequest(api.CreateMessageRequest{Messages: msgs})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(plain); strings.Contains(string(b), "chat_template_kwargs") {
		t.Errorf("an ordinary request carries chat_template_kwargs: %s", b)
	}
	quiet, err := c.buildRequest(api.CreateMessageRequest{Messages: msgs, NoThinking: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(quiet)
	if !strings.Contains(string(b), `"chat_template_kwargs":{"enable_thinking":false}`) {
		t.Errorf("NoThinking not sent: %s", b)
	}
}
