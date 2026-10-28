package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

// TestShouldCompactUsesContextWindowNotMaxTokens guards against the bug where
// compaction was triggered off cfg.MaxTokens (the per-request output cap)
// instead of cfg.ContextWindow (the model's real total context length).
// For a small-context local model (e.g. 35000 tokens) with a modest MaxTokens
// (e.g. 8096), the old formula fired compaction at ~6072 input tokens —
// nowhere near the real 35000-token limit — while requests kept asking for
// up to 8096 output tokens on top of the current input, risking an actual
// overflow of the model's real window long before the old threshold reflected it.
func TestShouldCompactUsesContextWindowNotMaxTokens(t *testing.T) {
	cfg := &Config{
		CompactionEnabled:   true,
		CompactionThreshold: 0.75,
		MaxTokens:           8096,
		ContextWindow:       35000,
	}

	// Below the real threshold: (35000-8096)*0.75 ≈ 20178. Must not compact yet.
	if ShouldCompact(15000, nil, cfg) {
		t.Fatalf("compacted too early: 15000 input tokens is well under the 35k window")
	}

	// At/above the real threshold: must compact.
	if !ShouldCompact(21000, nil, cfg) {
		t.Fatalf("failed to compact: 21000 input tokens should trip the 35k-window threshold")
	}
}

func TestShouldCompactDisabled(t *testing.T) {
	cfg := &Config{CompactionEnabled: false, ContextWindow: 35000, MaxTokens: 8096}
	if ShouldCompact(9999999, nil, cfg) {
		t.Fatalf("compaction must never trigger when disabled")
	}
}

// TestShouldCompactCountsToolResultsSinceLastReport guards against the bug where
// the last reported prompt size, taken before a tool ran, hid the tool's result:
// a big file read appended after that report never counted, so the next request
// overflowed the window instead of compacting first.
func TestShouldCompactCountsToolResultsSinceLastReport(t *testing.T) {
	cfg := &Config{
		CompactionEnabled:   true,
		CompactionThreshold: 0.75,
		MaxTokens:           8096,
		ContextWindow:       32768,
	}
	page := strings.Repeat("x", 20000*charsPerToken) // ~20k tokens of file/page content
	msgs := []api.Message{{Role: "user", Content: []api.ContentBlock{{
		Type:    "tool_result",
		Content: []api.ContentBlock{{Type: "text", Text: page}},
	}}}}

	// Last report was 5k tokens, from before the read: the 20k must still count.
	if !ShouldCompact(5000, msgs, cfg) {
		t.Fatalf("failed to compact: a ~20k-token tool result should trip the 32k-window threshold")
	}
}

// TestRetainRecentLeavesRoomUnderThreshold guards against compaction thrash: the
// last few messages were kept whatever their size, so a couple of big file reads
// left the history over the threshold right after compacting, and every round-trip
// compacted again - rewriting the system prompt and throwing away the whole cache.
func TestRetainRecentLeavesRoomUnderThreshold(t *testing.T) {
	cfg := &Config{
		CompactionEnabled:    true,
		CompactionThreshold:  0.75,
		CompactionKeepRecent: 10,
		MaxTokens:            8096,
		ContextWindow:        32768,
	}
	read := func(id string) []api.Message {
		return []api.Message{
			{Role: "assistant", Content: []api.ContentBlock{{Type: "tool_use", ID: id, Name: "read_file", Input: map[string]any{"path": id}}}},
			{Role: "user", Content: []api.ContentBlock{{Type: "tool_result", ToolUseID: id,
				Content: []api.ContentBlock{{Type: "text", Text: strings.Repeat("x", 12000*charsPerToken)}}}}},
		}
	}
	var msgs []api.Message
	for _, id := range []string{"a", "b", "c", "d"} {
		msgs = append(msgs, read(id)...)
	}

	recent := retainRecent(msgs, cfg)
	if ShouldCompact(0, recent, cfg) {
		t.Fatalf("retained %d tokens, still over the threshold: the next round-trip compacts again", EstimateTokens(recent))
	}
	if len(recent) < 2 || recent[0].Role != "assistant" || recent[len(recent)-1].Content[0].ToolUseID != "d" {
		t.Fatalf("expected the newest call and its result kept as a pair, got %d messages", len(recent))
	}
	if len(msgs[len(msgs)-1].Content[0].Content[0].Text) != 12000*charsPerToken {
		t.Fatalf("retainRecent modified the caller's messages")
	}
}

// TestCompactionCarriesSummaryForward: each compaction summarized only the messages
// left since the last one, so what the earlier summary covered was lost.
func TestCompactionCarriesSummaryForward(t *testing.T) {
	got := buildTranscript("earlier: edited internal/runtime/compact.go", nil, 300)
	if !strings.Contains(got, "earlier: edited internal/runtime/compact.go") {
		t.Fatalf("previous summary missing from the transcript")
	}
}

// summaryClient plays a model that answers each summary request with the next
// numbered summary and one fact, and a fold request with "folded".
type summaryClient struct {
	n        int
	requests []api.CreateMessageRequest
}

func (c *summaryClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.requests = append(c.requests, req)
	text := "folded"
	if !strings.HasPrefix(req.Messages[len(req.Messages)-1].Content[0].Text, foldInstruction[:40]) {
		c.n++
		text = fmt.Sprintf("worked on step %d\nFACTS:\n- decided option %d", c.n, c.n)
	}
	ch := make(chan api.StreamEvent, 8)
	defer close(ch)
	ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: text}}
	ch <- api.StreamEvent{Type: api.EventContentBlockStop}
	ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
	ch <- api.StreamEvent{Type: api.EventMessageStop}
	return ch, nil
}

func say(role, text string) api.Message {
	return api.Message{Role: role, Content: []api.ContentBlock{{Type: "text", Text: text}}}
}

// TestCompactionKeepsEarlyFacts: every compaction re-summarized the summary
// before it, so a detail from early on had to survive being chosen again each
// time - and an email address given on the second turn was gone, or misspelled,
// a few compactions later. The address is kept word for word, and every
// earlier section stays as it was written.
func TestCompactionKeepsEarlyFacts(t *testing.T) {
	client := &summaryClient{}
	loop := NewConversationLoop(&Config{Model: "test-model", MaxTokens: 1024, ContextWindow: 40960, CompactionKeepRecent: 2}, client)
	loop.Session.Messages = []api.Message{
		say("user", "send it to soandso@gmail.com, and the doc is at https://example.com/a/b."),
		say("assistant", "ok"),
	}
	for i := 0; i < 12; i++ {
		loop.Session.Messages = append(loop.Session.Messages, say("user", fmt.Sprintf("next %d", i)), say("assistant", "done"))
		if _, err := loop.ManualCompact(context.Background()); err != nil {
			t.Fatalf("compact %d: %v", i, err)
		}
	}
	summary := loop.Session.CompactionSummary
	for _, want := range []string{"soandso@gmail.com", "https://example.com/a/b", "worked on step 1", "decided option 1", "worked on step 12"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lost %q after 12 compactions:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "https://example.com/a/b.") {
		t.Errorf("the link kept the sentence's full stop")
	}
	if n := strings.Count(loop.Session.Messages[0].Content[0].Text, continuationPrefix); n != 1 {
		t.Errorf("history opens with %d continuation markers, want 1", n)
	}
	for _, msg := range loop.Session.Messages[1:] {
		if strings.HasPrefix(msg.Content[0].Text, continuationPrefix) {
			t.Errorf("an earlier continuation message was kept: the model would read the summary twice")
		}
	}
}

// TestCompactionFoldsOnlyTheOldest: when the sections outgrow their budget, the
// oldest are folded into one, and the newest stays as written.
func TestCompactionFoldsOnlyTheOldest(t *testing.T) {
	client := &summaryClient{}
	cfg := &Config{Model: "test-model", MaxTokens: 1024, ContextWindow: 8192, CompactionThreshold: 0.75}
	big := strings.Repeat("word ", summaryBudget(cfg, 8)*charsPerToken/5/3+1)
	session := &Session{CompactionSections: []string{big, big, big, "newest"}}
	foldSections(context.Background(), client, cfg, session)
	got := session.CompactionSections
	if len(got) != 3 || got[0] != "folded" || got[len(got)-1] != "newest" {
		t.Fatalf("sections after folding: %d, first %.20q, last %.20q", len(got), got[0], got[len(got)-1])
	}
}

// TestCompactionMigratesOldSummary: a session saved before sections existed
// keeps its summary as the first section.
func TestCompactionMigratesOldSummary(t *testing.T) {
	client := &summaryClient{}
	loop := NewConversationLoop(&Config{Model: "test-model", MaxTokens: 1024, ContextWindow: 40960}, client)
	loop.Session.CompactionSummary = "the old summary"
	loop.Session.Messages = []api.Message{say("user", "hi"), say("assistant", "hello")}
	if _, err := loop.ManualCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := loop.Session.CompactionSections; len(s) != 2 || s[0] != "the old summary" {
		t.Fatalf("sections = %q", s)
	}
}

func TestSplitSummary(t *testing.T) {
	section, facts := splitSummary("did things\n\n**FACTS:**\n- a@b.co\n* path /x/y\nnot a fact\n- ")
	if section != "did things" || len(facts) != 2 || facts[0] != "a@b.co" || facts[1] != "path /x/y" {
		t.Fatalf("got %q %q", section, facts)
	}
	if section, facts := splitSummary("no facts here"); section != "no facts here" || facts != nil {
		t.Fatalf("got %q %q", section, facts)
	}
}
