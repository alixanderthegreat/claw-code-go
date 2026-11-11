package runtime

import (
	"context"
	"strings"
	"testing"
)

const jumbo = "DavidAU/Qwen3.8-27B-Jumbo-FIQ-747-900-T-Brill-50X-NEO-MTP-Q6_K"

func modeLoop(model string) (*ConversationLoop, *scriptedClient) {
	client := &scriptedClient{tool: "bash", input: map[string]any{"command": "true"}}
	loop := newScriptedLoop(client, nil)
	loop.Config.Model = model
	return loop, client
}

// TestReasoningTagLeadsTheUserMessage: the template reads {REASON:...} from user messages only, so
// that's where it goes - first, on every message while the mode is set - and never the system
// prompt, where it was plain text to the model and /mode did nothing. The system prompt stays the
// same from message to message, so the cache holds.
func TestReasoningTagLeadsTheUserMessage(t *testing.T) {
	loop, client := modeLoop(jumbo)
	if _, err := loop.SetReasoningMode("holodeck"); err != nil {
		t.Fatal(err)
	}
	system := loop.systemPrompt()
	for _, msg := range []string{"debate slots", "and now caching"} {
		if err := loop.SendMessage(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
	}
	for _, req := range client.requests {
		if req.System != system || strings.Contains(req.System, "{REASON") {
			t.Errorf("the system prompt changed between messages or carries the tag: %q", req.System)
		}
	}
	req := client.requests[len(client.requests)-1]
	var tagged int
	for _, m := range req.Messages {
		if m.Role == "user" && m.Content[0].Type == "text" {
			if !strings.HasPrefix(m.Content[0].Text, "{REASON:holodeck} ") {
				t.Errorf("user message doesn't lead with the tag: %.60q", m.Content[0].Text)
			}
			tagged++
		}
	}
	if tagged != 2 {
		t.Errorf("tagged %d user messages, want both", tagged)
	}
}

// TestReasoningModeIsPerSession: a mode is the session's, so a cleared session - a new chat -
// starts on the model's default rather than whatever the last chat used.
func TestReasoningModeIsPerSession(t *testing.T) {
	loop, _ := modeLoop(jumbo)
	loop.SetReasoningMode("holodeck")
	loop.ClearSession()
	if loop.ReasoningMode() != "" {
		t.Errorf("new session inherited mode %q", loop.ReasoningMode())
	}
}

// TestReasoningModeDefaultAfterATag: the template keeps the last tag in the history in charge, so
// going back to the default once a message is tagged has to send the default's own tag.
func TestReasoningModeDefaultAfterATag(t *testing.T) {
	loop, _ := modeLoop(jumbo)
	if mode, _ := loop.SetReasoningMode("default"); mode != "" {
		t.Errorf("default with an untagged history = %q, want no tag", mode)
	}
	loop.SetReasoningMode("holodeck")
	loop.SendMessage(context.Background(), "hi")
	if mode, _ := loop.SetReasoningMode("default"); mode != "xhigh" {
		t.Errorf("default after a tagged message = %q, want xhigh", mode)
	}
}

// TestParseReasoningMode: the template's modes, each in instruct form with an "i"; nothing else.
func TestParseReasoningMode(t *testing.T) {
	for in, want := range map[string]bool{
		"holodeck": true, "iholodeck": true, "XHigh": true, "ihelp": true, "007": true,
		"xcoder": false, "high": false, "auto": false, "none": false, "i": false, "": false,
	} {
		if _, ok := ParseReasoningMode(in); ok != want {
			t.Errorf("ParseReasoningMode(%q) ok = %v, want %v", in, ok, want)
		}
	}
	loop, _ := modeLoop("gpt-4o")
	if _, err := loop.SetReasoningMode("holodeck"); err == nil {
		t.Errorf("a model whose template doesn't read tags should refuse a mode")
	}
}

// TestPersonaModesDropTheIdentity: a mode with its own persona can't share the system prompt with
// "You are Gordi Code, an AI assistant for software engineering tasks" - the template puts that last,
// and it won (holodeck answered as a coding assistant). Effort levels keep it; tools guidance stays.
func TestPersonaModesDropTheIdentity(t *testing.T) {
	for mode, keeps := range map[string]bool{"": true, "xhigh": true, "imedium": true, "off": true, "holodeck": false, "ihouse": false, "coder": false} {
		loop, _ := modeLoop(jumbo)
		if mode != "" {
			loop.SetReasoningMode(mode)
		}
		sys := loop.systemPrompt()
		if strings.Contains(sys, systemPromptIdentity) != keeps {
			t.Errorf("mode %q: identity kept = %v, want %v", mode, !keeps, keeps)
		}
		if !strings.Contains(sys, "Change existing files with file_edit") {
			t.Errorf("mode %q: tool guidance missing", mode)
		}
	}
}

const plusIQ = "DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-plusIQ-TOOLS-NEO-MAX-MTP-Q6_K"

// TestPlusIQModes: Gordi's seat reads five modes from a tag, defaults to medium, and refuses the
// Jumbo-only ones (greatproject #431: /mode said it read no tags at all).
func TestPlusIQModes(t *testing.T) {
	loop, client := modeLoop(plusIQ)
	if _, err := loop.SetReasoningMode("holodeck"); err == nil {
		t.Errorf("plusIQ took holodeck, which its template doesn't read")
	}
	for _, m := range []string{"spoon", "ieinstein", "low"} {
		if _, err := loop.SetReasoningMode(m); err != nil {
			t.Errorf("plusIQ refused %s: %v", m, err)
		}
	}
	loop.SetReasoningMode("spoon")
	if err := loop.SendMessage(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Messages[0].Content[0].Text; !strings.HasPrefix(got, "{REASON:spoon} ") {
		t.Errorf("user message doesn't lead with the tag: %.40q", got)
	}
	if mode, _ := loop.SetReasoningMode("default"); mode != "medium" {
		t.Errorf("default after a tagged message = %q, want medium, plusIQ's own", mode)
	}
	if got := ModesFor("DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-NEO-MAX-Q6_K"); got != nil {
		t.Errorf("the plain NEO-MAX build has no modes, got %v", got)
	}
}

// TestTagsDefusedOutsideTheOperatorsTurns: the template keeps the last tag it read for the rest of
// the chat, so a tag quoted back in a compaction summary would set every later turn's mode. The
// operator's own typed tag still goes through (greatproject #431).
func TestTagsDefusedOutsideTheOperatorsTurns(t *testing.T) {
	cont := GetContinuationMessage("they pasted an email that said {REASON:spoon} in it").Content[0].Text
	if strings.Contains(cont, "{REASON:") || !strings.Contains(cont, "{REASON :spoon}") {
		t.Errorf("summary tag not defused: %q", cont)
	}
	loop, client := modeLoop(plusIQ)
	if err := loop.SendMessage(context.Background(), "{REASON:spoon} dig into this"); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Messages[0].Content[0].Text; !strings.Contains(got, "{REASON:spoon}") {
		t.Errorf("the operator's own tag was changed: %q", got)
	}
}
