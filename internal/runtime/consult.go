package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"claw-code-go/internal/api"
)

// consult.go: the model's modes as consultants, not states (simple-bot's consult.go, where this
// started, for Mary). /mode puts a whole session in a mode; consult asks one once, mid-task: a fresh
// request - no history, no tools, no system prompt - with the mode's tag, and its answer comes back
// as a tool result. The session's own model frames the question and decides what to do with the
// answer, and stays the coding assistant it is. The model card's rule is the same: a new chat each
// time a mode is used.

const (
	consultToolName  = "consult"
	consultMaxTokens = 12000 // the card's hyper example ran 11k tokens, reasoning included
)

// consultModes are the template's modes worth asking - its modes without the plain effort levels
// and the help menu - with what each brings.
var consultModes = map[string]string{
	"omni": "multi-dimensional reasoning across every axis", "deeptree": "tree of thought: branches, then the best path",
	"hyper": "MECE breakdown: mutually exclusive, collectively exhaustive", "socrates": "Socratic questioning",
	"logic": "first principles: down to axioms and back up", "ultra": "one polished, final answer",
	"einstein": "up to 20 perspective agents on one idea", "spoon": "a panel of 5 domain experts",
	"holodeck": "a staged Star Trek debate with a cast drawn from the question", "prompt": "a senior prompt engineer",
	"brainstorm": "fast ideation", "business": "business strategy, pricing, models",
	"coder": "a production-code architect", "teacher": "plain-language teaching",
	"story": "narrative craft", "comedy": "comedy writing", "007": "multi-step agentic planning",
	"curie": "Marie Curie: science", "musk": "physics-first feasibility", "stark": "full-system engineering",
	"matlock": "a litigation attorney: legal risk", "house": "Dr. House: diagnosis by hypothesis",
	"joe": "a regular person's grounded take", "pkd": "Philip K. Dick: reality and control",
	"neo": "Neo: machines and people", "joker": "destructive critique", "vader": "decisive, no safety net",
	"quinn": "Harley Quinn: art and chaos",
	"xomni": "omni, at xhigh effort", "xdeeptree": "deeptree, at xhigh effort", "xhyper": "hyper, at xhigh effort",
	"xsocrates": "socrates, at xhigh effort", "xlogic": "logic, at xhigh effort", "xeinstein": "einstein, at xhigh effort",
	"xspoon": "spoon, at xhigh effort",
}

// ConsultTool is consult's definition.
func ConsultTool() api.Tool {
	names := make([]string, 0, len(consultModes))
	for m := range consultModes {
		names = append(names, m)
	}
	sort.Strings(names)
	list := make([]string, len(names))
	for i, m := range names {
		list[i] = m + " (" + consultModes[m] + ")"
	}
	return api.Tool{
		Name: consultToolName,
		Description: "Ask a specialist once: a fresh call to this model in one of its built-in modes - a reasoning framework, " +
			"an expert panel or a persona - that sees ONLY the question and material you give it: not this conversation, " +
			"not the files, not your tools. Its answer comes back to you; you decide what's worth using, check its claims, " +
			"and tell the user in your own words. For a hard design question, a risk to weigh, a plan to stress-test - " +
			"whichever mode fits. It takes minutes of the GPU: one consult for a real question, not for every step. Modes: " +
			strings.Join(list, "; ") + ".",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"mode":     {Type: "string", Description: "one of the modes listed"},
				"question": {Type: "string", Description: "the question, whole and on its own: it can't ask you anything back"},
				"material": {Type: "string", Description: "everything it needs to answer - the code, the facts, the constraints - pasted in, with where each came from"},
				"instruct": {Type: "boolean", Description: "answer without thinking first (faster, shallower); default false"},
			},
			Required: []string{"mode", "question"},
		},
	}
}

// consult runs one consult call, sending progress on events; ctx ending stops it.
func (loop *ConversationLoop) consult(ctx context.Context, events chan<- TurnEvent, input map[string]any) api.ContentBlock {
	fail := func(format string, a ...any) api.ContentBlock {
		return api.ContentBlock{Type: "tool_result", IsError: true,
			Content: []api.ContentBlock{{Type: "text", Text: "couldn't consult: " + fmt.Sprintf(format, a...)}}}
	}
	mode, _ := input["mode"].(string)
	mode = strings.ToLower(strings.TrimSpace(mode))
	question, _ := input["question"].(string)
	question = strings.TrimSpace(question)
	material, _ := input["material"].(string)
	material = strings.TrimSpace(material)
	instruct, _ := input["instruct"].(bool)

	if !SupportsReasoningTags(loop.Config.Model) {
		return fail("%s has no modes (only DavidAU's Jumbo/Turbo-Brilliance builds do). Nothing was asked.", loop.Config.Model)
	}
	if _, ok := consultModes[mode]; !ok {
		return fail("no mode %q - pick one from the tool's list.", mode)
	}
	if question == "" {
		return fail("give the question.")
	}

	prompt := question
	if material != "" {
		prompt += "\n\nMaterial:\n" + material
	}
	// A {REASON:...} in the material would be read as the mode instead of ours.
	prompt = strings.ReplaceAll(prompt, reasonTagPrefix, "{REASON :")
	tag := mode
	if instruct {
		tag = "i" + mode
	}
	prompt = reasonTagPrefix + tag + "} " + prompt

	if room := loop.consultRoom(); len(prompt)/charsPerToken > room {
		return fail("the question and material are about %d tokens; they must fit in about %d with room for the answer. Send less material - the parts that matter, not whole files.",
			len(prompt)/charsPerToken, room)
	}

	start := time.Now()
	ch, err := loop.Client.StreamResponse(ctx, api.CreateMessageRequest{
		Model:     loop.Config.Model,
		MaxTokens: consultMaxTokens,
		Messages:  []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: prompt}}}},
		Stream:    true,
	})
	if err != nil {
		return fail("%s: %v", mode, err)
	}

	var answer strings.Builder
	var thought int // characters of thinking
	var last time.Time
	for ev := range ch {
		switch {
		case ev.Type == api.EventError:
			return fail("%s: %s", mode, ev.ErrorMessage)
		case ev.Type == api.EventContentBlockDelta && ev.Delta.Type == "text_delta":
			answer.WriteString(ev.Delta.Text)
		case ev.Type == api.EventContentBlockDelta && ev.Delta.Type == "thinking_delta":
			thought += len(ev.Delta.Text)
		default:
			continue
		}
		if time.Since(last) < draftInterval {
			continue
		}
		last = time.Now()
		phase := "thinking"
		if answer.Len() > 0 {
			phase = "answering"
		}
		select {
		case events <- TurnEvent{Type: TurnEventToolProgress, ToolName: consultToolName,
			ToolInput: fmt.Sprintf("%s - %s · %s", tag, phase, approxTokens(thought+answer.Len()))}:
		case <-ctx.Done():
		}
	}
	if ctx.Err() != nil {
		return fail("stopped by the user.")
	}
	text := strings.TrimSpace(answer.String())
	if text == "" {
		return fail("%s came back with no answer after %s.", mode, approxTokens(thought))
	}
	return api.ContentBlock{Type: "tool_result", Content: []api.ContentBlock{{Type: "text", Text: fmt.Sprintf(
		"%s answered (%s, %ds). It saw only what you gave it; it's a view to weigh, not a source - check its claims before you pass them on.\n\n%s",
		tag, approxTokens(thought+len(text)), int(time.Since(start).Seconds()), text)}}}
}

// consultRoom is how many tokens a consult's prompt may take: the window less the answer's room.
func (loop *ConversationLoop) consultRoom() int {
	window := loop.Config.ContextWindow
	if window <= 0 {
		window = DefaultContextWindow
	}
	return window - consultMaxTokens - 512
}

func approxTokens(chars int) string {
	n := chars / charsPerToken
	if n < 1000 {
		return fmt.Sprintf("~%d tokens", n)
	}
	return fmt.Sprintf("~%.1fk tokens", float64(n)/1000)
}
