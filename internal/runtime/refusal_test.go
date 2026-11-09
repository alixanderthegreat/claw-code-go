package runtime

import (
	"strings"
	"testing"

	"claw-code-go/internal/permissions"
)

// TestRefusalText: what the model reads when a call is refused (#30). The wording is free; these
// are what it has to carry.
func TestRefusalText(t *testing.T) {
	push := &permissions.Rule{Tool: "bash", Pattern: "git push", Decision: permissions.DecisionDeny}

	byRule := refusalText(refusedByRule, "bash", push)
	if !strings.Contains(byRule, `"git push"`) {
		t.Errorf("a rule's refusal doesn't name its pattern: %q", byRule)
	}
	if blocked := refusalText(refusedByRule, "web_fetch", nil); !strings.Contains(blocked, "web_fetch") {
		t.Errorf("a blocked tool's refusal doesn't name the tool: %q", blocked)
	}

	// Each kind of refusal reads differently, so the model can tell a fence from an absent
	// operator from the operator's "no".
	texts := map[string]refusedBy{}
	for _, by := range []refusedBy{refusedByRule, refusedUnattended, refusedByOperator} {
		txt := refusalText(by, "bash", push)
		if prev, ok := texts[txt]; ok {
			t.Errorf("refusals %d and %d read the same: %q", prev, by, txt)
		}
		texts[txt] = by
	}
}
