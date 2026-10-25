package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"claw-code-go/internal/api"
)

const (
	// DefaultCompactionThreshold triggers compaction when input tokens reach
	// this fraction of MaxTokens (e.g., 0.75 = 75%).
	DefaultCompactionThreshold = 0.75

	// DefaultCompactionKeepRecent is the number of recent messages retained
	// verbatim after compaction.
	DefaultCompactionKeepRecent = 10

	// charsPerToken is the approximate character-to-token ratio used for estimation.
	charsPerToken = 4
)

// CompactionState tracks token usage and compaction history across turns.
type CompactionState struct {
	LastInputTokens   int // input tokens from the most recently completed turn
	TotalInputTokens  int // cumulative input tokens across all turns
	TotalOutputTokens int // cumulative output tokens across all turns
	CompactionCount   int // number of times the session has been compacted
}

// EstimateTokens roughly estimates the number of tokens in a slice of messages
// using a simple chars-per-token heuristic.
func EstimateTokens(messages []api.Message) int {
	var total int
	for _, msg := range messages {
		for _, cb := range msg.Content {
			total += len(cb.Text) / charsPerToken
			if len(cb.Input) > 0 {
				// Tool-call arguments are resent too, and a write or edit
				// carries a whole file's worth of them.
				if b, err := json.Marshal(cb.Input); err == nil {
					total += len(b) / charsPerToken
				}
			}
			if cb.Type == "image" {
				total += api.ImageTokens
			}
			for _, inner := range cb.Content {
				total += len(inner.Text) / charsPerToken
				if inner.Type == "image" {
					total += api.ImageTokens
				}
			}
		}
	}
	return total
}

// ShouldCompact returns true when the session should be compacted.
// It uses the actual API-reported input token count when available (> 0),
// falling back to EstimateTokens.
//
// The threshold is a fraction of cfg.ContextWindow (the model's total context
// length), reserved for the per-request output budget (cfg.MaxTokens) — NOT
// a fraction of cfg.MaxTokens itself. MaxTokens is only the output cap sent
// on each request; conflating the two means compaction is measured against
// the wrong number entirely, which is especially dangerous for small-context
// local models where MaxTokens can be a large fraction of (or even exceed)
// the model's real window.
func ShouldCompact(inputTokens int, messages []api.Message, cfg *Config) bool {
	if !cfg.CompactionEnabled {
		return false
	}
	// The reported count is from the previous request, so it leaves out the
	// tool results appended since - a file or page read can be tens of
	// thousands of tokens. Take whichever is larger so that growth counts.
	if est := EstimateTokens(messages); est > inputTokens {
		inputTokens = est
	}
	return inputTokens >= compactionThreshold(cfg)
}

// compactionThreshold is the input size, in tokens, at which ShouldCompact fires.
func compactionThreshold(cfg *Config) int {
	budget := cfg.ContextWindow - cfg.MaxTokens
	if budget <= 0 {
		budget = cfg.ContextWindow
	}
	return int(float64(budget) * cfg.CompactionThreshold)
}

// collectStreamText drains a StreamEvent channel and returns the concatenated
// text response, or an error if the stream encountered one.
func collectStreamText(ch <-chan api.StreamEvent) (string, error) {
	var sb strings.Builder
	for event := range ch {
		switch event.Type {
		case api.EventError:
			return "", fmt.Errorf("stream error: %s", event.ErrorMessage)
		case api.EventContentBlockDelta:
			if event.Delta.Type == "text_delta" {
				sb.WriteString(event.Delta.Text)
			}
		}
	}
	return sb.String(), nil
}

// buildTranscript constructs a plain-text transcript of the messages suitable
// for submission to the summarization model.
// A previous compaction's summary, if any, is written in whole ahead of them.
func buildTranscript(prevSummary string, messages []api.Message) string {
	var sb strings.Builder
	sb.WriteString("Please provide a concise but thorough summary of the following conversation. ")
	sb.WriteString("Preserve all important technical details: file paths modified, commands run, ")
	sb.WriteString("decisions made, errors encountered, and the current state of any ongoing work. ")
	sb.WriteString("The summary will replace the conversation history and must be self-contained.\n\n")
	if prevSummary != "" {
		sb.WriteString("The conversation was summarized before; fold that summary into yours.\n\n")
		sb.WriteString("---EARLIER SUMMARY---\n")
		sb.WriteString(prevSummary)
		sb.WriteString("\n---END EARLIER SUMMARY---\n\n")
	}
	sb.WriteString("---CONVERSATION---\n")

	for _, msg := range messages {
		fmt.Fprintf(&sb, "\n[%s]:\n", strings.ToUpper(msg.Role))
		for _, cb := range msg.Content {
			switch cb.Type {
			case "text":
				if len(cb.Text) > 1000 {
					sb.WriteString(cb.Text[:1000])
					sb.WriteString("... [truncated]\n")
				} else {
					sb.WriteString(cb.Text)
					sb.WriteString("\n")
				}
			case "image":
				sb.WriteString("[An image]\n")
			case "tool_use":
				fmt.Fprintf(&sb, "[Tool call: %s]\n", cb.Name)
			case "tool_result":
				for _, inner := range cb.Content {
					if len(inner.Text) > 300 {
						fmt.Fprintf(&sb, "[Tool result: %s... (truncated)]\n", inner.Text[:300])
					} else {
						fmt.Fprintf(&sb, "[Tool result: %s]\n", inner.Text)
					}
				}
			}
		}
	}
	sb.WriteString("\n---END CONVERSATION---\n")
	return sb.String()
}

// compactSummaryMaxTokens leaves room for a reasoning model to think before it
// writes the summary: thinking counts against the cap but never reaches it.
const compactSummaryMaxTokens = 4096

// compactInstruction is the turn added to the end of the conversation to ask
// for its summary.
const compactInstruction = "[System: This conversation is about to be compacted to stay within context limits. " +
	"Write a concise but thorough summary of it so far, to replace the history. Preserve all important technical " +
	"details: file paths read or modified, commands run, decisions made, errors encountered, and the current state " +
	"of any ongoing work, including what was about to happen next. It must be self-contained. Reply with the " +
	"summary as plain text only - do not call any tools.]"

// CompactSession summarizes the session's message history by calling the model,
// stores the summary in the session, and trims the message list to what
// retainRecent keeps. Returns the summary text.
//
// It asks for the summary at the end of the conversation itself, sent with the
// same system prompt and tools as every turn, so the server serves the whole
// history from its prompt cache and only the request is new - and the summary
// is written from the full tool results, not a cut-down transcript. A separate
// transcript prompt shared no prefix with anything: it was prefilled from
// scratch and took a cache entry from another caller (Mary's, on 2026-10-23).
// The transcript stays as the fallback for when that request fails or comes
// back without text, e.g. a model that answered with a tool call.
func CompactSession(ctx context.Context, client api.APIClient, cfg *Config, session *Session, system string, tools []api.Tool) (string, error) {
	if len(session.Messages) == 0 {
		return "", nil
	}

	messages := make([]api.Message, len(session.Messages), len(session.Messages)+1)
	copy(messages, session.Messages)
	messages = append(messages, api.Message{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: compactInstruction}}})
	summary, err := requestSummary(ctx, client, api.CreateMessageRequest{
		Model:     cfg.Model,
		MaxTokens: compactSummaryMaxTokens,
		System:    system,
		Messages:  messages,
		Tools:     tools,
		Stream:    true,
	})
	if err != nil || strings.TrimSpace(summary) == "" {
		// The previous summary goes in whole: the history being summarized only
		// starts where it left off, so leaving it out (or truncating it like any
		// other long text) would drop everything it covered.
		transcript := buildTranscript(session.CompactionSummary, session.Messages)
		summary, err = requestSummary(ctx, client, api.CreateMessageRequest{
			Model:     cfg.Model,
			MaxTokens: compactSummaryMaxTokens,
			Messages:  []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: transcript}}}},
			Stream:    true,
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(summary) == "" {
			return "", fmt.Errorf("compact: the model returned no summary")
		}
	}

	session.CompactionSummary = summary
	session.CompactionCount++
	session.Messages = retainRecent(session.Messages, cfg)

	return summary, nil
}

func requestSummary(ctx context.Context, client api.APIClient, req api.CreateMessageRequest) (string, error) {
	ch, err := client.StreamResponse(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compact: stream response: %w", err)
	}
	summary, err := collectStreamText(ch)
	if err != nil {
		return "", fmt.Errorf("compact: collect stream: %w", err)
	}
	return summary, nil
}

// retainRecent picks the messages kept verbatim after compaction: at most
// cfg.CompactionKeepRecent, and no more than half the compaction threshold in
// tokens. A count alone kept whatever the last few messages held - a couple of
// file or page reads could leave the history over the threshold right after
// compacting, so every round-trip compacted again and threw the cache away.
// Tool results too big for that budget are cut down to their start.
func retainRecent(messages []api.Message, cfg *Config) []api.Message {
	budget := compactionThreshold(cfg) / 2
	start, used := len(messages), 0
	for start > 0 && len(messages)-start < cfg.CompactionKeepRecent {
		msg := trimToolResults(messages[start-1], budget/4)
		n := EstimateTokens([]api.Message{msg})
		if used+n > budget && start < len(messages) {
			break
		}
		used += n
		start--
	}
	// A tool result can't be sent without the call it answers, so keep that
	// call too, over budget or not - dropping the result instead would lose
	// the newest tool output in the middle of a task.
	if start > 0 && start < len(messages) && isToolResults(messages[start]) {
		start--
	}
	recent := make([]api.Message, 0, len(messages)-start)
	for _, msg := range messages[start:] {
		recent = append(recent, trimToolResults(msg, budget/4))
	}
	return recent
}

func isToolResults(msg api.Message) bool {
	for _, cb := range msg.Content {
		if cb.Type != "tool_result" {
			return false
		}
	}
	return len(msg.Content) > 0
}

// trimToolResults returns msg with any tool result text over maxTokens cut
// down to its start, noting how much was dropped. msg itself is not changed.
func trimToolResults(msg api.Message, maxTokens int) api.Message {
	limit := maxTokens * charsPerToken
	out := msg
	out.Content = make([]api.ContentBlock, len(msg.Content))
	for i, cb := range msg.Content {
		if cb.Type == "tool_result" {
			inner := make([]api.ContentBlock, len(cb.Content))
			for j, in := range cb.Content {
				if len(in.Text) > limit {
					in.Text = fmt.Sprintf("%s\n... [%d more characters dropped when the conversation was compacted; run the tool again if you need them]",
						in.Text[:limit], len(in.Text)-limit)
				}
				inner[j] = in
			}
			cb.Content = inner
		}
		out.Content[i] = cb
	}
	return out
}

// GetContinuationMessage creates a synthetic user message that announces the
// compaction event, suitable for prepending to the retained recent messages.
func GetContinuationMessage(summary string) api.Message {
	text := fmt.Sprintf(
		"[System: Conversation history was automatically compacted to stay within context limits.\n\nSummary of prior context:\n%s\n\nContinuing from here.]",
		summary,
	)
	return api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "text", Text: text},
		},
	}
}
