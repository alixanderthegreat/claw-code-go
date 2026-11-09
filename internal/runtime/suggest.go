package runtime

import (
	"context"
	"regexp"
	"strings"

	"claw-code-go/internal/api"
)

// suggestAsk is the instruction added after the conversation for a next-prompt suggestion (#32).
const suggestAsk = "[Not from the operator: a side request from the interface.] " +
	"Write the operator's most likely next message to you, as they would type it: one short line, " +
	"their words, not yours - no quotes, no explanation. If you can't tell, write nothing. " +
	"Don't call a tool."

// thinkRE is a reasoning model's thinking, when a server leaves it in the text.
var thinkRE = regexp.MustCompile(`(?s)<think>.*?</think>`)

// suggestMaxTokens is enough for one line: the call asks for no thinking (a reasoning model
// given 400 tokens spent them all thinking and wrote nothing, 2026-11-09).
const suggestMaxTokens = 80

// SuggestNext asks the model for the operator's likely next message after the turn that just
// ended, or "" when it has none. The request is the turn's own - the same system prompt, tools
// and messages - with one instruction added at the end, so a server that caches by prefix
// (kronk) reuses the whole conversation and reads only the new tokens. messages is a copy taken
// when the turn ended; the loop's own history is not touched.
func (loop *ConversationLoop) SuggestNext(ctx context.Context, messages []api.Message) (string, error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
		return "", nil
	}
	req := api.CreateMessageRequest{
		Model:     loop.Config.Model,
		MaxTokens: suggestMaxTokens,
		System:    loop.systemPrompt(),
		Messages: append(messages[:len(messages):len(messages)], api.Message{
			Role:    "user",
			Content: []api.ContentBlock{{Type: "text", Text: suggestAsk}},
		}),
		Tools:      loop.allTools(),
		Stream:     true,
		NoThinking: true,
	}
	ch, err := loop.Client.StreamResponse(ctx, req)
	if err != nil {
		return "", err
	}
	text, err := collectStreamText(ch)
	if err != nil {
		return "", err
	}
	return cleanSuggestion(text), nil
}

// cleanSuggestion is the reply's first non-empty line, without quotes or a "Operator:" label, at
// most 200 characters; "" when that leaves nothing.
func cleanSuggestion(s string) string {
	s = thinkRE.ReplaceAllString(s, "")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, p := range []string{"Operator:", "operator:", "User:", "You:"} {
			line = strings.TrimSpace(strings.TrimPrefix(line, p))
		}
		line = strings.Trim(line, "\"'`“”")
		if r := []rune(line); len(r) > 200 {
			line = string(r[:200])
		}
		return strings.TrimSpace(line)
	}
	return ""
}
