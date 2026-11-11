package runtime

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"claw-code-go/internal/api"
)

// suggestAsk is the instruction added after the conversation for a next-prompt suggestion (#32).
const suggestAsk = "[Not from the operator: a side request from the interface.] " +
	"Write the operator's most likely next message to the assistant, as the operator would type it: " +
	"one line of at most 20 words, in the operator's voice, not the assistant's - don't answer or " +
	"carry on the assistant's reply. No quotes, no explanation. If you can't tell, write nothing. " +
	"Don't call a tool."

// thinkRE is a reasoning model's thinking, when a server leaves it in the text.
var thinkRE = regexp.MustCompile(`(?s)<think>.*?</think>`)

// suggestMaxTokens is enough for one line: the call asks for no thinking (a reasoning model
// given 400 tokens spent them all thinking and wrote nothing, 2026-11-09).
const suggestMaxTokens = 80

// What the suggestion reads (#32, greatproject #224): the end of the last reply and the operator's
// last few messages, not the conversation. Resending the whole turn to reuse a server's cached
// prefix didn't work: the suggestion asks without thinking, the turns think, and a template that
// renders past turns differently in the two modes leaves only the system prompt cached - the
// suggestion read ~27k tokens to write one line, and pushed the conversation out of the cache the
// next turn wanted. A short context costs little on any model, and fits a small one.
const (
	suggestReplyChars = 3000 // the end of the last reply
	suggestUserChars  = 600  // the end of each operator message: their words come after the context prepended to it
	suggestUserTurns  = 3
)

const suggestSystem = "You guess what a person will type next to their assistant, from the end of their conversation. " +
	"Answer with one short line in their words, or nothing if you can't tell."

// SuggestNext asks for the operator's likely next message after the turn that just ended, or ""
// when there is none. It asks Config.SuggestModel when set (a small model is enough), else the main
// model, with a short context (suggestContext) and no tools. messages is a copy taken when the turn
// ended; the loop's own history is not touched.
func (loop *ConversationLoop) SuggestNext(ctx context.Context, messages []api.Message) (string, error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
		return "", nil
	}
	model := loop.Config.Model
	if loop.Config.SuggestModel != "" {
		model = loop.Config.SuggestModel
	}
	req := api.CreateMessageRequest{
		Model:     model,
		MaxTokens: suggestMaxTokens,
		System:    suggestSystem,
		Messages: []api.Message{{
			Role:    "user",
			Content: []api.ContentBlock{{Type: "text", Text: DefuseReasonTags(suggestContext(messages)) + "\n\n" + suggestAsk}},
		}},
		Stream:     true,
		NoThinking: true,
	}
	ch, err := loop.Client.StreamResponse(ctx, req)
	if err != nil {
		return "", err
	}
	text, cut, err := collectSuggestion(ch)
	if err != nil || cut {
		return "", err // a suggestion cut off by the token limit is never shown half-written (#60)
	}
	return cleanSuggestion(text), nil
}

// collectSuggestion is the reply's text, and whether the server stopped it at the token limit.
func collectSuggestion(ch <-chan api.StreamEvent) (text string, cut bool, err error) {
	var sb strings.Builder
	for event := range ch {
		switch event.Type {
		case api.EventError:
			err = errors.New(event.ErrorMessage)
		case api.EventContentBlockDelta:
			if event.Delta.Type == "text_delta" {
				sb.WriteString(event.Delta.Text)
			}
		case api.EventMessageDelta:
			if event.StopReason == "max_tokens" || event.Usage.FinishReason == "length" {
				cut = true
			}
		}
	}
	return sb.String(), cut, err
}

// suggestContext is what the suggestion reads: the operator's last few messages (their text only,
// the end of each, oldest first) and the end of the assistant's last reply.
func suggestContext(messages []api.Message) string {
	var users []string
	for i := len(messages) - 1; i >= 0 && len(users) < suggestUserTurns; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if t := strings.TrimSpace(blockText(messages[i])); t != "" {
			users = append(users, tail(t, suggestUserChars))
		}
	}
	var b strings.Builder
	b.WriteString("The operator's last messages to the assistant, oldest first:\n")
	for i := len(users) - 1; i >= 0; i-- {
		b.WriteString("<<<\n" + users[i] + "\n>>>\n")
	}
	b.WriteString("\nThe end of the assistant's last reply to the operator (the assistant's words, not the operator's):\n<<<\n" +
		tail(strings.TrimSpace(blockText(messages[len(messages)-1])), suggestReplyChars) + "\n>>>")
	return b.String()
}

// blockText is a message's text blocks, joined: not tool calls or their results.
func blockText(m api.Message) string {
	var parts []string
	for _, c := range m.Content {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// tail is the last n runes of s, marked when cut.
func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "[...] " + string(r[len(r)-n:])
}

// suggestMaxChars is the longest suggestion shown. Longer, it isn't one line the operator would
// type: it's the model carrying on the conversation, and cutting it showed half a sentence (#60).
const suggestMaxChars = 200

// cleanSuggestion is the reply's first non-empty line, without quotes or a "Operator:" label; ""
// when that leaves nothing or the line runs past suggestMaxChars.
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
		line = strings.TrimSpace(strings.Trim(line, "\"'`“”"))
		if len([]rune(line)) > suggestMaxChars {
			return ""
		}
		return line
	}
	return ""
}
