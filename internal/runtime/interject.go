package runtime

import (
	"strings"
	"sync"

	"claw-code-go/internal/api"
)

// interject.go: messages the user sends while a turn runs. They used to wait for the turn to end,
// however many tool calls that took; now each goes to the model with the next round's tool
// results, as Claude Code delivers a message sent mid-turn. One the model never got to - the turn
// ended on that round's answer - the TUI sends as the next message (TakeInterjections).

type interjections struct {
	mu    sync.Mutex
	texts []string
}

// Interject queues text for the model's next step. Safe to call while a turn runs.
func (loop *ConversationLoop) Interject(text string) {
	loop.interjected.mu.Lock()
	loop.interjected.texts = append(loop.interjected.texts, text)
	loop.interjected.mu.Unlock()
}

// TakeInterjections returns the messages not yet given to the model, and clears them.
func (loop *ConversationLoop) TakeInterjections() []string {
	loop.interjected.mu.Lock()
	defer loop.interjected.mu.Unlock()
	texts := loop.interjected.texts
	loop.interjected.texts = nil
	return texts
}

// interjectionBlock is the text block that carries pending messages into a round's tool results,
// or nil when there are none.
func (loop *ConversationLoop) interjectionBlock() []api.ContentBlock {
	texts := loop.TakeInterjections()
	if len(texts) == 0 {
		return nil
	}
	loop.stats.interjections += len(texts)
	return []api.ContentBlock{{Type: "text", Text: "[The user sent this while you were working - take it into account now:]\n\n" + strings.Join(texts, "\n\n")}}
}
