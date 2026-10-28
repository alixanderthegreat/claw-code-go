package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claw-code-go/internal/api"
	"claw-code-go/internal/permissions"
)

// scriptedClient plays a model that makes one tool call and then ends the
// turn - or, with repeat, makes the same call on every response, the way a
// model stuck on a bad call does. It records every request so a test can
// inspect what the loop sent back: the tool_result the model would actually
// see, and how big the next request got.
type scriptedClient struct {
	tool     string
	input    map[string]any
	repeat   bool
	fail     int // fail this many requests before answering, like an overflow
	requests []api.CreateMessageRequest
}

// maxScriptedRequests stops a repeating script that the loop never stops, so
// that bug fails the test instead of hanging it.
const maxScriptedRequests = 10

func (c *scriptedClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.requests = append(c.requests, req)
	if len(c.requests) <= c.fail {
		return nil, fmt.Errorf("input tokens exceed context window")
	}
	ch := make(chan api.StreamEvent, 8)
	defer close(ch)

	last := req.Messages[len(req.Messages)-1]
	if isSummaryRequest(req) {
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: "summary"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
		ch <- api.StreamEvent{Type: api.EventMessageStop}
		return ch, nil
	}
	calling := last.Content[0].Type != "tool_result" || c.repeat
	if calling && len(c.requests) <= maxScriptedRequests {
		raw, _ := json.Marshal(c.input)
		id := fmt.Sprintf("call_%d", len(c.requests))
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "tool_use", ID: id, Name: c.tool}}
		ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "input_json_delta", PartialJSON: string(raw)}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "tool_use"}
	} else {
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: "done"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
	}
	ch <- api.StreamEvent{Type: api.EventMessageStop}
	return ch, nil
}

// isSummaryRequest reports whether req is compaction asking for a summary, in
// place or as a transcript.
func isSummaryRequest(req api.CreateMessageRequest) bool {
	last := req.Messages[len(req.Messages)-1].Content[0]
	return last.Type == "text" && (strings.HasPrefix(last.Text, compactInstructionPrefix) || strings.Contains(last.Text, "---CONVERSATION---"))
}

// toolResult returns the tool_result the loop sent back for the scripted call.
func (c *scriptedClient) toolResult(t *testing.T) api.ContentBlock {
	t.Helper()
	for _, req := range c.requests {
		for _, msg := range req.Messages {
			for _, cb := range msg.Content {
				if cb.Type == "tool_result" && cb.ToolUseID == "call_1" {
					return cb
				}
			}
		}
	}
	t.Fatalf("no tool_result for call_1 was ever sent to the model")
	return api.ContentBlock{}
}

// entryPoints are the two ways a turn gets driven: SendMessage backs -prompt
// (and so every serve dispatch), SendMessageStreaming backs the TUI. The TUI
// runner returns an error event as its error, the way the TUI shows it.
var entryPoints = map[string]func(*ConversationLoop) error{
	"prompt": func(loop *ConversationLoop) error {
		return loop.SendMessage(context.Background(), "go")
	},
	"tui": func(loop *ConversationLoop) error {
		events := make(chan TurnEvent)
		done := make(chan error, 1)
		go func() { done <- loop.SendMessageStreaming(context.Background(), "go", events) }()
		var shown error
		for {
			select {
			case err := <-done:
				if err != nil {
					return err
				}
				return shown
			case ev := <-events:
				if ev.PermReply != nil {
					ev.PermReply <- PermDecisionAllowOnce
				}
				if ev.Type == TurnEventError {
					shown = ev.Err
				}
			}
		}
	},
}

func newScriptedLoop(client *scriptedClient, rules *permissions.Ruleset) *ConversationLoop {
	loop := NewConversationLoop(&Config{Model: "test-model", MaxTokens: 1024, ContextWindow: 40960}, client)
	loop.CtxAssembler = nil
	loop.PermManager = permissions.NewManager(permissions.ModeBypassPermissions, rules)
	return loop
}

// TestDenyListHoldsOnEveryEntryPoint: serve dispatch runs bypass mode with
// web_fetch on the deny list. The deny has to hold no matter which entry point
// drives the turn - item 0 of kata cycle 0 watched -prompt fetch anyway.
func TestDenyListHoldsOnEveryEntryPoint(t *testing.T) {
	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetched = true
		fmt.Fprint(w, "fetched")
	}))
	defer srv.Close()

	for name, run := range entryPoints {
		t.Run(name, func(t *testing.T) {
			fetched = false
			client := &scriptedClient{tool: "web_fetch", input: map[string]any{"url": srv.URL}}
			loop := newScriptedLoop(client, permissions.RulesetFromLists(nil, []string{"web_fetch"}))

			if err := run(loop); err != nil {
				t.Fatalf("turn failed: %v", err)
			}
			if fetched {
				t.Errorf("web_fetch reached the network despite the deny rule")
			}
			if r := client.toolResult(t); !r.IsError {
				t.Errorf("expected a denied tool_result, got %q", r.Content[0].Text)
			}
		})
	}
}

// TestOversizedToolResultStillFits: a glob over a big tree once returned
// 855 KB (~200k tokens) to a 40960-token model, and every later request in
// that session failed. Whatever a tool returns, the request that carries it
// back must still fit the context window.
func TestOversizedToolResultStillFits(t *testing.T) {
	dir := t.TempDir()
	for i := range 3000 {
		name := filepath.Join(dir, fmt.Sprintf("file_with_a_fairly_long_name_%04d.go", i))
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	for name, run := range entryPoints {
		t.Run(name, func(t *testing.T) {
			client := &scriptedClient{tool: "glob", input: map[string]any{"path": dir, "pattern": "**/*.go"}}
			loop := newScriptedLoop(client, nil)

			if err := run(loop); err != nil {
				t.Fatalf("turn failed: %v", err)
			}
			text := client.toolResult(t).Content[0].Text
			if !strings.Contains(text, "file_with_a_fairly_long_name_0000.go") {
				t.Fatalf("glob result lost its content entirely: %.200q", text)
			}
			budget := loop.Config.ContextWindow - loop.Config.MaxTokens
			for i, req := range client.requests {
				if got := EstimateTokens(req.Messages); got > budget {
					t.Errorf("request %d carries ~%d tokens, over the %d-token budget", i, got, budget)
				}
			}
		})
	}
}

// TestInvalidInputRejectedOnEveryEntryPoint: a call missing a required field
// comes back as an error naming the field, so the model can correct itself.
func TestInvalidInputRejectedOnEveryEntryPoint(t *testing.T) {
	for name, run := range entryPoints {
		t.Run(name, func(t *testing.T) {
			client := &scriptedClient{tool: "bash", input: map[string]any{}}
			if err := run(newScriptedLoop(client, nil)); err != nil {
				t.Fatalf("turn failed: %v", err)
			}
			r := client.toolResult(t)
			if !r.IsError || !strings.Contains(r.Content[0].Text, "missing required input fields") {
				t.Errorf("expected a missing-field error, got %q", r.Content[0].Text)
			}
		})
	}
}

// TestFailureLimitStopsEveryEntryPoint: a model that keeps making the same
// invalid call - one per response - must be stopped with an error after three
// tries, not left to loop until something outside gives up on it.
func TestFailureLimitStopsEveryEntryPoint(t *testing.T) {
	for name, run := range entryPoints {
		t.Run(name, func(t *testing.T) {
			client := &scriptedClient{tool: "bash", input: map[string]any{}, repeat: true}
			err := run(newScriptedLoop(client, nil))
			if err == nil {
				t.Errorf("expected the failure limit to end the turn with an error")
			}
			if n := len(client.requests); n != 3 {
				t.Errorf("model was asked %d times, want exactly 3", n)
			}
		})
	}
}

// TestCompactionFiresAfterFailedRequest: the only token count on hand is from
// an earlier, smaller history. The history itself must be judged, so the
// message compacts before sending - and when the in-place summary request
// fails, as an overflowing one would, the transcript fallback still gets it
// done, rather than the stale count leaving the history to fail forever.
func TestCompactionFiresAfterFailedRequest(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "true"}, fail: 1}
	loop := newScriptedLoop(client, nil)
	loop.Config.CompactionEnabled = true
	loop.Config.CompactionThreshold = DefaultCompactionThreshold
	loop.Config.CompactionKeepRecent = 2
	big := strings.Repeat("x", 4*loop.Config.ContextWindow)
	loop.Session.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: big}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "ok"}}},
	}
	loop.Compaction.LastInputTokens = 1000 // from before the history grew

	if err := loop.SendMessage(context.Background(), "first"); err != nil {
		t.Fatalf("message failed: %v", err)
	}
	if loop.Compaction.CompactionCount != 1 {
		t.Errorf("history was never compacted after the failed attempt")
	}
}

// TestCompactionSummarizesInPlace: the summary is asked for at the end of the
// conversation, sent with the turn's own system prompt and tools, so the server
// serves the whole history from its prompt cache - and the system prompt is the
// same after compacting, so that shared prefix stays cached too.
func TestCompactionSummarizesInPlace(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "true"}}
	loop := newScriptedLoop(client, nil)
	loop.Session.Messages = []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "read the file"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "ok"}}},
	}
	history := len(loop.Session.Messages)
	system := loop.systemPrompt()

	if _, err := loop.ManualCompact(context.Background()); err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("made %d requests, want one in-place summary request", len(client.requests))
	}
	req := client.requests[0]
	if req.System != system || len(req.Tools) != len(loop.allTools()) {
		t.Errorf("summary request didn't carry the turn's own system prompt and tools - no shared prefix to reuse")
	}
	if len(req.Messages) != history+1 || !strings.HasPrefix(req.Messages[history].Content[0].Text, compactInstructionPrefix) {
		t.Errorf("summary request should be the history plus one instruction, got %d messages", len(req.Messages))
	}
	if loop.systemPrompt() != system {
		t.Errorf("system prompt changed on compaction: everything after it has to be prefilled again")
	}
	if !strings.Contains(loop.Session.Messages[0].Content[0].Text, "summary") {
		t.Errorf("summary missing from the continuation message")
	}
}

// TestCompactionFallsBackWhenSummaryIsAToolCall: a model can answer the
// in-place request with a tool call instead of text. That's no summary, so the
// transcript prompt - which offers no tools - is tried instead.
func TestCompactionFallsBackWhenSummaryIsAToolCall(t *testing.T) {
	client := &toolFirstSummaryClient{}
	loop := NewConversationLoop(&Config{Model: "test-model", MaxTokens: 1024, ContextWindow: 40960}, client)
	loop.Session.Messages = []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}}}

	summary, err := loop.ManualCompact(context.Background())
	if err != nil || summary != "transcript summary" {
		t.Fatalf("got %q, %v; want the transcript fallback's summary", summary, err)
	}
	if len(client.requests) != 2 || len(client.requests[1].Tools) != 0 {
		t.Fatalf("want an in-place request then a tool-less transcript request, got %d", len(client.requests))
	}
}

// toolFirstSummaryClient answers a request that offers tools with a tool call,
// and one that doesn't with text.
type toolFirstSummaryClient struct{ requests []api.CreateMessageRequest }

func (c *toolFirstSummaryClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.requests = append(c.requests, req)
	ch := make(chan api.StreamEvent, 8)
	defer close(ch)
	if len(req.Tools) > 0 {
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "tool_use", ID: "c1", Name: "bash"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "tool_use"}
	} else {
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: "transcript summary"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
	}
	ch <- api.StreamEvent{Type: api.EventMessageStop}
	return ch, nil
}

// TestFailedCommandKeepsItsOutput: a failed command's result is what it printed and then the error -
// "exit status 128" alone hid the "fatal: not a git repository" that said what was wrong.
func TestFailedCommandKeepsItsOutput(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "echo 'fatal: not a git repository'; exit 128"}}
	if err := newScriptedLoop(client, nil).SendMessage(context.Background(), "push"); err != nil {
		t.Fatal(err)
	}
	r := client.toolResult(t)
	text := r.Content[0].Text
	if !r.IsError || !strings.HasPrefix(text, "fatal: not a git repository") || !strings.Contains(text, "exit status 128") {
		t.Errorf("result = %q", text)
	}
}

// TestAskUserNeedsNoPermission: asking the user isn't something to ask them permission for - even
// in default mode the question goes straight to them.
func TestAskUserNeedsNoPermission(t *testing.T) {
	client := &scriptedClient{tool: "ask_user", input: map[string]any{"question": "HTTP or SSH?"}}
	loop := newScriptedLoop(client, nil)
	loop.PermManager = permissions.NewManager(permissions.ModeDefault, nil)
	events := make(chan TurnEvent, 16)
	go func() {
		for ev := range events {
			switch ev.Type {
			case TurnEventPermissionAsk:
				t.Errorf("asked permission to ask the user")
				ev.PermReply <- PermDecisionAllowOnce
			case TurnEventAskUser:
				ev.AskUserReply <- "HTTP"
			}
		}
	}()
	if err := loop.SendMessageStreaming(context.Background(), "push", events); err != nil {
		t.Fatal(err)
	}
	close(events)
}
