package runtime

import (
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
	got := buildTranscript("earlier: edited internal/runtime/compact.go", nil)
	if !strings.Contains(got, "earlier: edited internal/runtime/compact.go") {
		t.Fatalf("previous summary missing from the transcript")
	}
}
