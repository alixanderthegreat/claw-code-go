package runtime

import (
	"strings"
	"testing"
)

// TestRuleInputIsWhole: permission rules see the whole command, so a pattern past the 300
// characters SummarizeToolInput keeps (a long command ending in "git push") still matches.
func TestRuleInputIsWhole(t *testing.T) {
	cmd := strings.Repeat("echo padding; ", 30) + "git push origin main"
	in := map[string]any{"command": cmd}
	if got := RuleInput(in); got != cmd {
		t.Errorf("RuleInput cut the command: %d of %d characters", len(got), len(cmd))
	}
	if strings.Contains(SummarizeToolInput(in), "git push") {
		t.Fatal("test premise: the summary should have cut off the push")
	}
}
