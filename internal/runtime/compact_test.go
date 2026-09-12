package runtime

import "testing"

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
