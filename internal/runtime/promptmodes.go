package runtime

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// promptmodes.go: the modes a model's template can't read from a tag, given as a system prompt
// instead (greatproject #441) - the way consult runs joker on the 9B. plusIQ's template takes five
// modes from a tag; the other thirty are as much Gordi's as consult's.

// modePrompts are consult's mode prompts (simple-bot consult/modeprompts.json, #351): each mode's
// instructions, rendered from the 27B's chat template. A copy: regenerate it with consult's, never
// edit it by hand.
//
//go:embed modeprompts.json
var modePromptsJSON []byte

var modePrompts = func() map[string]string {
	m := map[string]string{}
	if err := json.Unmarshal(modePromptsJSON, &m); err != nil {
		panic("runtime: modeprompts.json: " + err.Error())
	}
	return m
}()

// introduceTask is a mode prompt's own "[Task] Introduce yourself..." line: with it, the model
// introduced itself instead of doing the work (consult da31e1, #438).
var introduceTask = regexp.MustCompile(`(?m)^.*\[Task\][^\n]*Introduce yourself[^\n]*\n?`)

// promptModes are the modes model gets as a system prompt: every consult mode its template doesn't
// read from a tag, sorted.
func promptModes(model string) []string {
	tags, _ := modesFor(model)
	inTags := map[string]bool{}
	for _, t := range tags {
		inTags[t] = true
	}
	var out []string
	for m := range modePrompts {
		if !inTags[m] {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// isPromptMode reports whether mode (with or without the instruct "i") reaches model as a system
// prompt rather than a tag.
func isPromptMode(model, mode string) bool {
	key := strings.TrimPrefix(mode, "i")
	if tags, _ := modesFor(model); tags != nil {
		if _, ok := parseMode(mode, tags); ok {
			return false
		}
	}
	_, ok := modePrompts[key]
	if !ok {
		_, ok = modePrompts[mode] // a mode whose own name starts with "i"
	}
	return ok
}

// promptModeText is the session's mode as a system prompt, "" when the mode is a tag or none.
func (loop *ConversationLoop) promptModeText() string {
	mode := loop.ReasoningMode()
	if mode == "" || !isPromptMode(loop.Config.Model, mode) {
		return ""
	}
	p, ok := modePrompts[strings.TrimPrefix(mode, "i")]
	if !ok {
		p = modePrompts[mode]
	}
	return strings.TrimSpace(introduceTask.ReplaceAllString(p, ""))
}

// promptModeInstruct is whether the session's prompt mode is in instruct form: thinking off.
func (loop *ConversationLoop) promptModeInstruct() bool {
	mode := loop.ReasoningMode()
	_, plain := modePrompts[mode]
	return isPromptMode(loop.Config.Model, mode) && strings.HasPrefix(mode, "i") && !plain
}
