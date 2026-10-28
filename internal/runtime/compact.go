package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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
// for submission to the summarization model, asking for a summary of about
// words words. A previous compaction's summary, if any, is written in whole
// ahead of them, as context: it is kept as it is, so the new summary covers
// only what came after it.
func buildTranscript(prevSummary string, messages []api.Message, words int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Summarize the following conversation in at most %d words. ", words)
	sb.WriteString("Preserve the important details: file paths modified, commands run, ")
	sb.WriteString("decisions made, errors encountered, and the current state of any ongoing work. ")
	sb.WriteString("The summary will replace the conversation history and must be self-contained. ")
	sb.WriteString(factsRequest + "\n\n")
	if prevSummary != "" {
		sb.WriteString("The conversation was summarized before. That summary is kept as it is: don't repeat it, ")
		sb.WriteString("summarize only the conversation after it.\n\n")
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

// compactInstructionPrefix opens the turn added to the end of the conversation
// to ask for its summary; compactInstruction completes it.
const compactInstructionPrefix = "[System: This conversation is about to be compacted to stay within context limits. "

// factsRequest asks for the exact details to carry forward, under a FACTS:
// line that splitSummary finds.
const factsRequest = "After the summary, write a line that says FACTS: and then, one per line starting with \"- \", " +
	"every exact detail worth keeping word for word: email addresses, names, numbers, ids, URLs, file paths, dates, " +
	"decisions made and what the user asked for. Copy each one exactly as it was written. Leave out any already " +
	"listed in the earlier summary."

// compactInstruction asks for a summary of about words words of everything
// after the earlier summary, which is kept as it is.
//
// Asking for a "concise" summary of the whole history, earlier summary
// included, made every fact from before it compete again for a shrinking
// space: one session's summary went from 513 tokens to 290 over 28
// compactions, and three days of work came down to its last task.
func compactInstruction(words int) string {
	return compactInstructionPrefix + fmt.Sprintf("Summarize it in at most %d words, to replace the history. ", words) +
		"If it begins with a summary of earlier history, that summary is kept as it is: don't repeat it, summarize " +
		"only what came after it. Preserve the important details: file paths read or modified, commands run, " +
		"decisions made, errors encountered, and the current state of any ongoing work, including what was about to " +
		"happen next. It must be self-contained. " + factsRequest + " Reply with plain text only - do not call any tools.]"
}

// summaryWords sizes a summary to the history it replaces: about a tenth of
// its tokens, as words, within bounds.
func summaryWords(messages []api.Message) int {
	words := EstimateTokens(messages) * 3 / 40
	return min(max(words, 150), 600)
}

// CompactSession summarizes the session's message history by calling the model,
// adds the summary to the session's earlier ones, and trims the message list to
// what retainRecent keeps. Returns the whole summary, as the model will see it.
//
// It asks for the summary at the end of the conversation itself, sent with the
// same system prompt and tools as every turn, so the server serves the whole
// history from its prompt cache and only the request is new - and the summary
// is written from the full tool results, not a cut-down transcript. A separate
// transcript prompt shared no prefix with anything: it was prefilled from
// scratch and took a cache entry from another caller (Mary's, on 2026-10-23).
// The transcript stays as the fallback for when that request fails or comes
// back without text, e.g. a model that answered with a tool call.
//
// Each compaction adds a section and never rewrites the earlier ones, and the
// exact details (CompactionFacts) are carried word for word. The addresses and
// links the user typed are copied by code, so they survive even when the model
// leaves them out or misspells them.
func CompactSession(ctx context.Context, client api.APIClient, cfg *Config, session *Session, system string, tools []api.Tool) (string, error) {
	if len(session.Messages) == 0 {
		return "", nil
	}
	// A session saved before sections existed carries its summary as one.
	if len(session.CompactionSections) == 0 && session.CompactionSummary != "" {
		session.CompactionSections = []string{session.CompactionSummary}
	}

	history := withoutContinuation(session.Messages)
	words := summaryWords(history)
	messages := make([]api.Message, len(session.Messages), len(session.Messages)+1)
	copy(messages, session.Messages)
	messages = append(messages, api.Message{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: compactInstruction(words)}}})
	summary, err := requestSummary(ctx, client, api.CreateMessageRequest{
		Model:     cfg.Model,
		MaxTokens: compactSummaryMaxTokens,
		System:    system,
		Messages:  messages,
		Tools:     tools,
		Stream:    true,
	})
	if err != nil || strings.TrimSpace(summary) == "" {
		// The previous summary goes in whole, as context for where this part
		// of the history starts.
		transcript := buildTranscript(session.CompactionSummary, history, words)
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

	section, facts := splitSummary(summary)
	recent := retainRecent(history, cfg)
	facts = append(facts, typedDetails(history[:len(history)-len(recent)])...)
	if section != "" {
		session.CompactionSections = append(session.CompactionSections, section)
	}
	session.CompactionFacts = addFacts(session.CompactionFacts, facts)
	foldSections(ctx, client, cfg, session)
	session.CompactionFacts = capFacts(session.CompactionFacts, cfg)

	session.CompactionSummary = renderSummary(session.CompactionSections, session.CompactionFacts)
	session.CompactionCount++
	session.Messages = recent

	return session.CompactionSummary, nil
}

// continuationPrefix opens the message GetContinuationMessage writes.
const continuationPrefix = "[System: Conversation history was automatically compacted"

// withoutContinuation returns messages without the continuation messages of
// earlier compactions: their summary is already in the session's sections,
// and one kept among the recent messages would put it in front of the model twice.
func withoutContinuation(messages []api.Message) []api.Message {
	out := make([]api.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "user" && len(msg.Content) == 1 && msg.Content[0].Type == "text" &&
			strings.HasPrefix(msg.Content[0].Text, continuationPrefix) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

// splitSummary splits a summary at its FACTS: line into the summary itself
// and the facts listed after it, one per line.
func splitSummary(text string) (string, []string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	at := -1
	for i, line := range lines {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(line), "*#: "), "facts") {
			at = i
		}
	}
	if at < 0 {
		return strings.TrimSpace(text), nil
	}
	var facts []string
	for _, line := range lines[at+1:] {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if fact := strings.TrimSpace(line[2:]); fact != "" {
				facts = append(facts, fact)
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines[:at], "\n")), facts
}

var (
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+`)
	urlPattern   = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
)

// typedDetails returns the email addresses and links the user typed in
// messages, exactly as typed - the strings a model is likeliest to retype wrong.
func typedDetails(messages []api.Message) []string {
	var out []string
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		for _, cb := range msg.Content {
			if cb.Type != "text" {
				continue
			}
			for _, url := range urlPattern.FindAllString(cb.Text, -1) {
				out = append(out, strings.TrimRight(url, ".,;:"))
			}
			for _, email := range emailPattern.FindAllString(urlPattern.ReplaceAllString(cb.Text, " "), -1) {
				out = append(out, email)
			}
		}
	}
	return out
}

// addFacts appends to facts each new one not already there, word for word.
func addFacts(facts, more []string) []string {
	seen := make(map[string]bool, len(facts))
	for _, f := range facts {
		seen[f] = true
	}
	for _, f := range more {
		if !seen[f] {
			seen[f] = true
			facts = append(facts, f)
		}
	}
	return facts
}

// summaryBudget is the most, in tokens, the summary's sections and its facts
// may each take: a share of the compaction threshold, so the summary can't
// crowd out the history it stands in front of.
func summaryBudget(cfg *Config, share int) int {
	return max(compactionThreshold(cfg)/share, 1000)
}

// capFacts keeps the newest facts that fit their budget.
func capFacts(facts []string, cfg *Config) []string {
	budget := summaryBudget(cfg, 16) * charsPerToken
	start, used := len(facts), 0
	for start > 0 && used+len(facts[start-1]) <= budget {
		start--
		used += len(facts[start])
	}
	return facts[start:]
}

// foldSections keeps the sections within their budget by folding the oldest
// ones into one shorter section. Only the oldest are ever summarized again; a
// newer section stays as it was written. If the model can't fold them, the
// folded text is cut to fit instead.
func foldSections(ctx context.Context, client api.APIClient, cfg *Config, session *Session) {
	budget := summaryBudget(cfg, 8) * charsPerToken
	total := 0
	for _, s := range session.CompactionSections {
		total += len(s)
	}
	if total <= budget || len(session.CompactionSections) < 2 {
		return
	}
	// Fold the oldest half, and at least two.
	n := max(len(session.CompactionSections)/2, 2)
	oldest := strings.Join(session.CompactionSections[:n], "\n\n")
	words := min(max(len(oldest)/charsPerToken*3/16, 150), 600)
	folded, err := requestSummary(ctx, client, api.CreateMessageRequest{
		Model:     cfg.Model,
		MaxTokens: compactSummaryMaxTokens,
		Messages: []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: fmt.Sprintf(
			foldInstruction+"\n\n---EARLIER SECTIONS---\n%s\n---END EARLIER SECTIONS---", words, oldest)}}}},
		Stream: true,
	})
	folded, _ = splitSummary(folded)
	if err != nil || folded == "" {
		folded = oldest
	}
	if limit := budget / 2; len(folded) > limit {
		folded = folded[:limit] + " [...]"
	}
	session.CompactionSections = append([]string{folded}, session.CompactionSections[n:]...)
}

// foldInstruction asks for the oldest sections of a summary as one, in at most
// %d words.
const foldInstruction = "These are the oldest sections of a running summary of a long conversation, oldest first. " +
	"Rewrite them as one section of at most %d words, keeping what still matters later: decisions, " +
	"what was built or changed and where, and anything left unfinished. Reply with the section only, as plain text."

// renderSummary writes the sections and facts as the model reads them.
func renderSummary(sections, facts []string) string {
	var sb strings.Builder
	for i, s := range sections {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(s)
	}
	if len(facts) > 0 {
		sb.WriteString("\n\nExact details from earlier, word for word:\n")
		for _, f := range facts {
			sb.WriteString("- " + f + "\n")
		}
	}
	return strings.TrimSpace(sb.String())
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
		continuationPrefix+" to stay within context limits.\n\nSummary of prior context:\n%s\n\nContinuing from here.]",
		summary,
	)
	return api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "text", Text: text},
		},
	}
}
