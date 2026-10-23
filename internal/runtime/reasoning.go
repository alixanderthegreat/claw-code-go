package runtime

import (
	"fmt"
	"strings"

	"claw-code-go/internal/api"
)

// reasoning.go: DavidAU's reasoning and instruct modes - {REASON:holodeck} - for the models whose
// chat template reads them (Jumbo, Turbo-Brilliance). The template, embedded in the GGUF, looks for
// the tag in USER messages only and the last tagged one wins: it strips the tag and puts that
// mode's instructions at the head of the system block. A tag in the system prompt is never read -
// it reaches the model as plain text, which is why /mode used to do nothing.
//
// A mode belongs to a session (the template's "current chat"), not to every session: it's kept on
// the Session, and while set, every user message starts with its tag. Tagging each message keeps
// the mode alive after compaction drops the one that set it, and leaves earlier messages byte-
// identical, so the prompt cache still holds. Changing mode changes the system block, so the next
// request after a change is prefilled in full once - the model card's own advice is a new chat per
// mode.

// ReasoningModes are the modes the template accepts, in its own order (reason_mods). Any of them
// with an "i" in front is the same mode in instruct form - thinking off.
var ReasoningModes = []string{
	"xomni", "xdeeptree", "xhyper", "xsocrates", "xlogic", "xhelp", "xeinstein", "xspoon",
	"omni", "deeptree", "hyper", "socrates", "logic", "off", "help", "einstein", "spoon", "ultra",
	"xhigh", "medium", "low",
	"holodeck", "prompt", "pkd", "musk", "joker", "vader", "quinn", "joe", "neo", "curie",
	"brainstorm", "comedy", "business", "story", "teacher", "007", "coder", "stark", "matlock", "house",
}

// defaultReasoningMode is what the template uses for reasoning when no message is tagged.
const defaultReasoningMode = "xhigh"

const reasonTagPrefix = "{REASON:"

// SupportsReasoningTags reports whether model's chat template reads {REASON:...} tags.
func SupportsReasoningTags(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "jumbo") || strings.Contains(m, "brill")
}

// ParseReasoningMode normalizes s to a mode the template accepts, with or without the instruct "i".
func ParseReasoningMode(s string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(s))
	key := mode
	if strings.HasPrefix(key, "i") && len(key) > 1 {
		key = key[1:]
	}
	for _, m := range ReasoningModes {
		if m == key || m == mode {
			return mode, true
		}
	}
	return "", false
}

// ReasoningMode returns the session's mode, empty for the model's default.
func (loop *ConversationLoop) ReasoningMode() string {
	if loop.Session == nil {
		return ""
	}
	return loop.Session.ReasoningMode
}

// SetReasoningMode sets the session's mode from s - a mode, or "default" - and returns the one now
// in effect. "default" when the history already carries a tag becomes xhigh, the template's own
// default: an untagged message would leave the last tag, and its mode, in charge.
func (loop *ConversationLoop) SetReasoningMode(s string) (string, error) {
	if !SupportsReasoningTags(loop.Config.Model) {
		return "", fmt.Errorf("%s doesn't read {REASON:...} tags - modes are for DavidAU's Jumbo/Turbo-Brilliance models", loop.Config.Model)
	}
	if strings.EqualFold(strings.TrimSpace(s), "default") {
		mode := ""
		if historyHasReasonTag(loop.Session.Messages) {
			mode = defaultReasoningMode
		}
		loop.Session.ReasoningMode = mode
		return mode, nil
	}
	mode, ok := ParseReasoningMode(s)
	if !ok {
		return "", fmt.Errorf("unknown mode %q - one of: %s (an \"i\" in front for instruct)", s, strings.Join(ReasoningModes, ", "))
	}
	loop.Session.ReasoningMode = mode
	return mode, nil
}

// nativeModes are Qwen's own effort levels and "off": they set how hard the model thinks, not who
// it is, so the identity in the system prompt stays.
var nativeModes = map[string]bool{"xhigh": true, "medium": true, "low": true, "off": true}

// modeReplacesIdentity reports whether the session's mode brings its own persona or framework. The
// template puts the mode's instructions first in the system block and the system prompt after the
// tool list, so "You are Gordi Code, an AI assistant for software engineering tasks" had the last
// word: with it, holodeck answered as a coding assistant; without it, as the debate it should be.
func (loop *ConversationLoop) modeReplacesIdentity() bool {
	if loop.reasoningTag() == "" {
		return false
	}
	return !nativeModes[strings.TrimPrefix(loop.ReasoningMode(), "i")]
}

// reasoningTag is the tag that starts the next user message, or "" for none.
func (loop *ConversationLoop) reasoningTag() string {
	mode := loop.ReasoningMode()
	if mode == "" || !SupportsReasoningTags(loop.Config.Model) {
		return ""
	}
	return reasonTagPrefix + mode + "}"
}

func historyHasReasonTag(messages []api.Message) bool {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		for _, cb := range msg.Content {
			if cb.Type == "text" && strings.Contains(cb.Text, reasonTagPrefix) {
				return true
			}
		}
	}
	return false
}
