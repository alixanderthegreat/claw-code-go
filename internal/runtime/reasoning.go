package runtime

import (
	"fmt"
	"strings"

	"claw-code-go/internal/api"
)

// reasoning.go: DavidAU's reasoning and instruct modes - {REASON:holodeck} - for the models whose
// chat template reads them (Jumbo, Turbo-Brilliance, and the plusIQ builds with a shorter list). The template, embedded in the GGUF, looks for
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

// PlusIQModes are the plusIQ builds' modes (Qwen3.5-9B The Defiant Fable plusIQ-TOOLS, Gordi's
// seat; greatproject #431): the template takes only these five from a tag, and defaults to medium.
var PlusIQModes = []string{"xhigh", "medium", "low", "einstein", "spoon"}

// modesFor is the modes model's template reads from a tag, and the one it uses when no message is
// tagged; nil for a template without modes.
func modesFor(model string) ([]string, string) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "plusiq"):
		return PlusIQModes, "medium"
	case strings.Contains(m, "jumbo") || strings.Contains(m, "brill"):
		return ReasoningModes, "xhigh"
	}
	return nil, ""
}

// ModesFor is every mode /mode takes on model: those its template reads from a {REASON:...} tag
// first, then consult's others, given as a system prompt (promptmodes.go, #441) - those only when
// consult answers.
func ModesFor(model string) []string {
	modes, _ := modesFor(model)
	return append(append([]string(nil), modes...), promptModes(model)...)
}

// DefuseReasonTags breaks every {REASON:...} in text the operator didn't write - a summary of the
// history, the history quoted back to the model - so the template doesn't read it as a mode
// switch. The template keeps the last tag it read for the rest of the chat, so one tag left in
// would set the mode of every turn after it (greatproject #431).
func DefuseReasonTags(text string) string {
	return strings.ReplaceAll(text, reasonTagPrefix, "{REASON :")
}

const reasonTagPrefix = "{REASON:"

// SupportsReasoningTags reports whether model's chat template reads {REASON:...} tags.
func SupportsReasoningTags(model string) bool {
	modes, _ := modesFor(model)
	return modes != nil
}

// ParseReasoningMode normalizes s to a mode Jumbo's template accepts, with or without the instruct "i".
func ParseReasoningMode(s string) (string, bool) {
	return parseMode(s, ReasoningModes)
}

// parseMode normalizes s to one of modes, with or without the instruct "i".
func parseMode(s string, modes []string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(s))
	key := mode
	if strings.HasPrefix(key, "i") && len(key) > 1 {
		key = key[1:]
	}
	for _, m := range modes {
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
// in effect. "default" when the history already carries a tag becomes the template's own default
// (xhigh, or medium for plusIQ): an untagged message would leave the last tag, and its mode, in charge.
func (loop *ConversationLoop) SetReasoningMode(s string) (string, error) {
	_, def := modesFor(loop.Config.Model)
	modes := ModesFor(loop.Config.Model)
	if strings.EqualFold(strings.TrimSpace(s), "default") {
		mode := ""
		if def != "" && historyHasReasonTag(loop.Session.Messages) {
			mode = def
		}
		loop.Session.ReasoningMode = mode
		return mode, nil
	}
	mode, ok := parseMode(s, modes)
	if ok {
		// A prompt mode is checked now, while you're there to hear it: consult has to answer.
		if name := promptModeName(loop.Config.Model, mode); name != "" {
			if _, err := consultModes.Prompt(name); err != nil {
				return "", err
			}
		}
	}
	if !ok {
		return "", fmt.Errorf("unknown mode %q - one of: %s (an \"i\" in front for instruct)", s, strings.Join(modes, ", "))
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
	if loop.promptModeText() != "" {
		return true
	}
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
	if promptModeName(loop.Config.Model, mode) != "" {
		// The persona is in the system prompt; the tag sets the template's own default effort, so
		// a tag earlier in the history (spoon) doesn't stay in charge under it. Its "i" turns the
		// thinking off for an instruct form.
		_, def := modesFor(loop.Config.Model)
		if loop.promptModeInstruct() {
			def = "i" + def
		}
		return reasonTagPrefix + def + "}"
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
