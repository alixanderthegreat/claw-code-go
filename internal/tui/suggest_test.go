package tui

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/api"
	"claw-code-go/internal/runtime"
)

func suggestModel(t *testing.T) (Model, *recordingClient) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	client := &recordingClient{}
	cfg := &runtime.Config{Model: "m", MaxTokens: 100, ContextWindow: 32768, SuggestNext: true}
	m := NewModel(cfg, runtime.NewConversationLoop(cfg, client))
	m.width, m.height = 100, 30
	return m, client
}

// The side call is the turn's own request - same system prompt and tools, the conversation as
// it was - with one instruction added, so kronk's prefix cache matches all of it.
func TestSuggestNextReusesTheTurn(t *testing.T) {
	m, client := suggestModel(t)
	m.loop.Session.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "build it"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "Built. Merge it?"}}},
	}
	before := len(m.loop.Session.Messages)
	got, err := m.loop.SuggestNext(context.Background(), m.loop.Session.Messages)
	if err != nil || got != "ok" {
		t.Fatalf("SuggestNext = %q, %v", got, err)
	}
	if len(m.loop.Session.Messages) != before {
		t.Error("the side call changed the conversation")
	}
	req := client.reqs[len(client.reqs)-1]
	if len(req.Messages) != before+1 || req.Messages[before].Role != "user" ||
		!strings.Contains(req.Messages[before].Content[0].Text, "next message") {
		t.Errorf("request messages: %+v", req.Messages)
	}
	if req.System != m.loop.SystemPrompt() || len(req.Tools) == 0 {
		t.Error("the side call's system prompt or tools differ from the turn's: kronk's cache won't match")
	}
	// Mid-turn (the last message isn't the model's): no call.
	m.loop.Session.Messages = m.loop.Session.Messages[:1]
	n := len(client.reqs)
	if got, _ := m.loop.SuggestNext(context.Background(), m.loop.Session.Messages); got != "" || len(client.reqs) != n {
		t.Error("asked for a suggestion with the operator's message last")
	}
}

func TestSuggestionShownAndTaken(t *testing.T) {
	m, _ := suggestModel(t)
	m.state = stateInput
	m = m.showSuggestion(suggestionMsg{gen: m.suggest.gen, text: "run the tests again"})
	if !strings.Contains(m.textarea.Placeholder, "run the tests again") {
		t.Fatalf("placeholder %q", m.textarea.Placeholder)
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(Model)
	if m.textarea.Value() != "run the tests again" || !m.suggest.taken {
		t.Fatalf("Tab: value %q, taken %v", m.textarea.Value(), m.suggest.taken)
	}
	m = m.endSuggestion("run the tests again")
	if m.suggest.text != "" || m.textarea.Placeholder != inputPlaceholder {
		t.Error("the suggestion outlived the send")
	}
	b, err := os.ReadFile(suggestionsLog())
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil || rec["outcome"] != "accepted" {
		t.Errorf("log %s: %v", b, err)
	}
	if st, _ := os.Stat(suggestionsLog()); st.Mode().Perm() != 0o600 {
		t.Errorf("log is %v, want 0600", st.Mode().Perm())
	}
}

func TestSuggestionNotShown(t *testing.T) {
	m, _ := suggestModel(t)
	m.state = stateInput
	// A late answer for an older turn.
	if got := m.showSuggestion(suggestionMsg{gen: m.suggest.gen - 1, text: "old"}); got.suggest.text != "" {
		t.Error("a stale suggestion was shown")
	}
	// The operator already started typing.
	m = typeText(m, "my own")
	if got := m.showSuggestion(suggestionMsg{gen: m.suggest.gen, text: "theirs"}); got.suggest.text != "" {
		t.Error("a suggestion was shown over what the operator typed")
	}
	// Off in the config: no side call at all.
	m.cfg.SuggestNext = false
	if m.suggestCmd() != nil {
		t.Error("suggest_next off, but a side call is made")
	}
}

func TestLogOutcomes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	logSuggestion("a", "a", true)
	logSuggestion("a", "a b", true)
	logSuggestion("a", "c", false)
	b, _ := os.ReadFile(suggestionsLog())
	var got []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var rec map[string]any
		json.Unmarshal([]byte(l), &rec) //nolint:errcheck
		got = append(got, rec["outcome"].(string))
	}
	if strings.Join(got, " ") != "accepted edited ignored" {
		t.Errorf("outcomes %v", got)
	}
}

// The trial (2026-11-09): every non-empty suggestion is offered, outward ones included; the log
// marks the outward ones.
func TestSuggestionAllowed(t *testing.T) {
	if suggestionAllowed("") {
		t.Error("an empty suggestion is offered")
	}
	for _, s := range []string{"run the tests again", "merge it", "push it to core"} {
		if !suggestionAllowed(s) {
			t.Errorf("%q held back during the trial", s)
		}
	}
}

func TestLooksOutward(t *testing.T) {
	for s, want := range map[string]bool{
		"merge em":                 true,
		"push it to core":          true,
		"deploy the worker":        true,
		"send it to Becky":         true,
		"git push origin main":     true,
		"ok, publish the page":     true,
		"post the requeue comment": true,
		"run the tests again":      false,
		"what does the board say?": false,
		"that's a pushover":        false,
		"the sender is wrong":      false,
	} {
		if got := looksOutward(s); got != want {
			t.Errorf("looksOutward(%q) = %v, want %v", s, got, want)
		}
	}
	t.Setenv("HOME", t.TempDir())
	logSuggestion("merge it", "merge it", true)
	b, _ := os.ReadFile(suggestionsLog())
	if !strings.Contains(string(b), `"outward":true`) {
		t.Errorf("the log doesn't mark an outward suggestion: %s", b)
	}
}
