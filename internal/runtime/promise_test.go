package runtime

import (
	"context"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

func text(s string) api.Message {
	return api.Message{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: s}}}
}

// TestEndsOnPromise: the ending Gordi gave on card #181 is a promise; a report, an offer or a
// question isn't (greatproject #435).
func TestEndsOnPromise(t *testing.T) {
	for s, want := range map[string]bool{
		"Card #181 is done. I've captured the decision. Next: move through the inbox. I'll pick #165 since it's due.": true,
		"Saved. Let me check the logs next.":                                  true,
		"Done: the record is at decisions/card-181.md, commit a1b2c3d.":       false,
		"I'll pick #165 next if you want.":                                    false,
		"Which card should I take next?":                                      false,
		"Let me know which one.":                                              false,
		"I'll start with the tests. Then I fixed it, and all tests pass: ok.": false,
	} {
		if got := endsOnPromise(text(s)); got != want {
			t.Errorf("endsOnPromise(%q) = %v, want %v", s, got, want)
		}
	}
}

// TestPromiseNudgedOnce: a turn that used a tool and ended on a promise is nudged once, and the
// nudge never repeats, so the loop can't run on.
func TestPromiseNudgedOnce(t *testing.T) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "true"}}
	loop := newScriptedLoop(client, nil)
	client.reply = "Recorded it. Next: I'll pick #165."
	if err := loop.SendMessage(context.Background(), "walk the cards"); err != nil {
		t.Fatal(err)
	}
	var nudges int
	for _, m := range loop.Session.Messages {
		if m.Role == "user" && len(m.Content) > 0 && strings.HasPrefix(m.Content[0].Text, "[claw-code-go] Your reply ends") {
			nudges++
		}
	}
	if nudges != 1 {
		t.Errorf("%d nudges, want 1", nudges)
	}
}
