package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const jumbo = "DavidAU/Qwen3.8-27B-Jumbo-FIQ-747-900-T-Brill-50X-NEO-MTP-Q6_K"

// fakeConsult stands in for consult's GET /modes and /modes/NAME/prompt (#441) for one test.
func fakeConsult(t *testing.T) {
	t.Helper()
	prompts := map[string]string{"joker": "You are The Joker.", "house": "You are Dr. House.", "holodeck": "A holodeck debate.", "spoon": "Qwen Deep Research"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/modes" {
			desc := map[string]string{}
			for k := range prompts {
				desc[k] = "a mode"
			}
			json.NewEncoder(w).Encode(desc)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/modes/"), "/prompt")
		if p, ok := prompts[name]; ok {
			w.Write([]byte(p))
			return
		}
		http.NotFound(w, r)
	}))
	consultModes.SetBase(srv.URL)
	t.Cleanup(func() { srv.Close(); consultModes.SetBase("") })
}

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
	fakeConsult(t)
	loop, _ := modeLoop("gpt-4o")
	if _, err := loop.SetReasoningMode("xhigh"); err == nil {
		t.Errorf("a model whose template doesn't read tags took xhigh, which is only a tag")
	}
	if _, err := loop.SetReasoningMode("holodeck"); err != nil {
		t.Errorf("holodeck is a prompt mode on any model (#441): %v", err)
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
	fakeConsult(t)
	loop, client := modeLoop(plusIQ)
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
	if got := ModesFor("DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-NEO-MAX-Q6_K"); len(got) == 0 || got[0] == "xhigh" {
		t.Errorf("the plain NEO-MAX build has prompt modes only, got %v", got)
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

// TestPlusIQPromptModes: on plusIQ, a mode its template doesn't read - joker - goes in the system
// prompt, less its "introduce yourself" task and with Gordi's identity dropped, while the message
// carries plusIQ's default effort tag so no earlier tag stays in charge. Its five stay tags
// (greatproject #441).
func TestPlusIQPromptModes(t *testing.T) {
	fakeConsult(t)
	loop, client := modeLoop(plusIQ)
	if _, err := loop.SetReasoningMode("ijoker"); err != nil {
		t.Fatal(err)
	}
	if err := loop.SendMessage(context.Background(), "what's next?"); err != nil {
		t.Fatal(err)
	}
	req := client.requests[0]
	if !strings.HasPrefix(req.System, "You are The Joker.") || strings.Contains(req.System, systemPromptIdentity) {
		t.Errorf("system prompt: joker first %v, Gordi identity %v", strings.HasPrefix(req.System, "You are The Joker."), strings.Contains(req.System, systemPromptIdentity))
	}
	if got := req.Messages[0].Content[0].Text; !strings.HasPrefix(got, "{REASON:imedium} ") || strings.Contains(got, "joker") {
		t.Errorf("message = %.40q, want plusIQ's default tag in instruct form", got)
	}
	modes := ModesFor(plusIQ)
	if modes[0] != "xhigh" || !strings.Contains(strings.Join(modes, ","), "joker") {
		t.Errorf("ModesFor(plusIQ) = %v, want its tags first, then joker and the rest", modes)
	}
	loop2, client2 := modeLoop(plusIQ)
	loop2.SetReasoningMode("spoon")
	loop2.SendMessage(context.Background(), "dig")
	if r := client2.requests[0]; !strings.HasPrefix(r.Messages[0].Content[0].Text, "{REASON:spoon} ") || strings.Contains(r.System, "Qwen Deep Research") {
		t.Errorf("spoon on plusIQ should stay a tag, not a system prompt")
	}
}

// TestPromptModeWithoutTags: on a model with no tag modes, an instruct prompt mode turns the
// thinking off in the request, and there's no tag at all.
func TestPromptModeWithoutTags(t *testing.T) {
	fakeConsult(t)
	loop, client := modeLoop("DavidAU/Qwen3.5-9B-The-Defiant-Fable-Uncnr-Heretic-NEO-MAX-Q6_K")
	if _, err := loop.SetReasoningMode("ihouse"); err != nil {
		t.Fatal(err)
	}
	loop.SendMessage(context.Background(), "diagnose this")
	req := client.requests[0]
	if !req.NoThinking || strings.Contains(req.Messages[0].Content[0].Text, "{REASON") || req.System == "" {
		t.Errorf("NoThinking %v, message %.30q", req.NoThinking, req.Messages[0].Content[0].Text)
	}
}

// TestPromptModeNeedsConsult: with consult out of reach, /mode joker says so and sets nothing;
// plusIQ's own tag modes still work (#441).
func TestPromptModeNeedsConsult(t *testing.T) {
	consultModes.SetBase("http://127.0.0.1:1")
	t.Cleanup(func() { consultModes.SetBase("") })
	loop, _ := modeLoop(plusIQ)
	if _, err := loop.SetReasoningMode("joker"); err == nil {
		t.Errorf("joker was set with consult unreachable")
	}
	if loop.ReasoningMode() != "" {
		t.Errorf("mode %q set anyway", loop.ReasoningMode())
	}
	if _, err := loop.SetReasoningMode("spoon"); err != nil {
		t.Errorf("spoon is a tag and needs no consult: %v", err)
	}
}
