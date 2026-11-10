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

// The side call reads a short context - the operator's last messages and the end of the last
// reply - with no tools, not the conversation (greatproject #224): resent whole, it missed the
// server's cache and read the whole conversation for one line.
func TestSuggestNextReadsAShortContext(t *testing.T) {
	m, client := suggestModel(t)
	long := strings.Repeat("x", 20000)
	m.loop.Session.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "first ask"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "tool_use", ID: "1", Name: "bash"}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "tool_result", ToolUseID: "1", Content: []api.ContentBlock{{Type: "text", Text: "TOOL OUTPUT"}}}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: long + " build it"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: long + " Built. Merge it?"}}},
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
	if len(req.Messages) != 1 || len(req.Tools) != 0 || req.Model != "m" {
		t.Fatalf("request: %d messages, %d tools, model %q", len(req.Messages), len(req.Tools), req.Model)
	}
	text := req.Messages[0].Content[0].Text
	for _, want := range []string{"first ask", "build it", "Merge it?", "next message"} {
		if !strings.Contains(text, want) {
			t.Errorf("context lacks %q", want)
		}
	}
	if strings.Contains(text, "TOOL OUTPUT") || len(text) > 6000 {
		t.Errorf("context carries tool output, or isn't short: %d chars", len(text))
	}
	// With suggest_model set, that model is asked.
	m.loop.Config.SuggestModel = "small"
	m.loop.SuggestNext(context.Background(), m.loop.Session.Messages)
	if req := client.reqs[len(client.reqs)-1]; req.Model != "small" {
		t.Errorf("asked %q, not the suggestion's model", req.Model)
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

// Every non-empty suggestion is offered while the log holds too few outward ones to decide; once
// it holds outward ones back (#50), inward ones are still offered. The gate notes itself once.
func TestSuggestionAllowed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defer func(g func() bool) { outwardHeld = g }(outwardHeld)
	outwardHeld = outwardGate(suggestionsLog)
	if suggestionAllowed("") {
		t.Error("an empty suggestion is offered")
	}
	for _, s := range []string{"run the tests again", "merge it", "push it to core"} {
		if !suggestionAllowed(s) {
			t.Errorf("%q held back with no log", s)
		}
	}

	for i := 0; i < 10; i++ {
		logSuggestion("push it", "no", false)
	}
	if suggestionAllowed("merge it") || suggestionAllowed("push it to core") {
		t.Error("an outward suggestion offered after 10 ignored")
	}
	if !suggestionAllowed("run the tests again") {
		t.Error("the gate held an inward suggestion")
	}
	b, _ := os.ReadFile(suggestionsLog())
	if n := strings.Count(string(b), "outward suggestions held: accept rate 0.0% over 10 entries"); n != 1 {
		t.Errorf("the gate noted itself %d times, want once: %s", n, b)
	}
}

// The gate follows the log within one process: 9 outward entries offer, the 10th crosses the
// threshold and the same gate holds from then on, noting it once.
func TestSuggestionGateCrossedMidProcess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defer func(g func() bool) { outwardHeld = g }(outwardHeld)
	outwardHeld = outwardGate(suggestionsLog)
	for i := 0; i < 9; i++ {
		logSuggestion("push it", "no", false)
	}
	if !suggestionAllowed("push it") {
		t.Fatal("held at 9 outward entries")
	}
	logSuggestion("push it", "no", false)
	for i := 0; i < 3; i++ {
		if suggestionAllowed("push it") {
			t.Fatal("offered after the 10th outward entry was logged")
		}
	}
	b, _ := os.ReadFile(suggestionsLog())
	if n := strings.Count(string(b), `"event"`); n != 1 {
		t.Errorf("the gate noted itself %d times, want once: %s", n, b)
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

// The side call asks for no thinking: given room to think, the 27B spent it all and wrote nothing.
func TestSuggestNextAsksNoThinking(t *testing.T) {
	m, client := suggestModel(t)
	m.loop.Session.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "build it"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "Built."}}},
	}
	if _, err := m.loop.SuggestNext(context.Background(), m.loop.Session.Messages); err != nil {
		t.Fatal(err)
	}
	if req := client.reqs[len(client.reqs)-1]; !req.NoThinking || req.MaxTokens > 100 {
		t.Errorf("side call: NoThinking %v, MaxTokens %d", req.NoThinking, req.MaxTokens)
	}
}
