package runtime

import (
	"strings"
	"testing"

	"claw-code-go/internal/permissions"
)

// With no one to ask (-prompt, serve's dispatch: bypass mode), a call an ask rule matches isn't
// run, and the model is told which rule wanted the operator (#3).
func TestAskRuleUnattendedIsRefusedByName(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "echo ran > /dev/null; ./deploy.sh"}}
	rules := &permissions.Ruleset{Rules: []permissions.Rule{{Tool: "bash", Pattern: "deploy", Decision: permissions.DecisionAsk}}}
	loop := newScriptedLoop(client, rules)
	if err := entryPoints["prompt"](loop); err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	r := client.toolResult(t)
	if !r.IsError || !strings.Contains(r.Content[0].Text, `bash calls containing "deploy"`) ||
		!strings.Contains(r.Content[0].Text, "no one was there to ask") {
		t.Errorf("tool result: %q", r.Content[0].Text)
	}
}
