package runtime

import (
	"time"

	"claw-code-go/internal/api"
)

// telemetry.go: the events Gordi sends about its own work (internal/telemetry), so Grafana can
// answer what kronk's per-request metrics can't - which requests made up a turn, where the turn's
// time went, which tools failed, what got retried. Counts, durations and names only: never a
// message, an argument or a result.
//
//	gordi_run   one model request: tokens (prompt, cached, output, thinking), MTP drafts,
//	            time to first output, duration, stop and finish reasons, kronk's failure if any
//	gordi_tool  one tool call: duration, failed, output trimmed, asked you, denied
//	gordi_turn  one exchange: requests, tools, and its time split into model, tools, and you
//	gordi_image one image given to the model: from your message or read_file, its size and type

// turnStats accumulates one exchange for its gordi_turn event.
type turnStats struct {
	start                  time.Time
	source                 string // "message", or "shell" for a ! command's output on its own
	rounds, tools, failed  int
	retries, compactions   int
	modelMs, toolMs, youMs int64
}

func (loop *ConversationLoop) startTurnStats(userText string) {
	source := "message"
	if userText == "" {
		source = "shell"
	}
	loop.stats = turnStats{start: time.Now(), source: source}
}

func (loop *ConversationLoop) emitRun(start, firstOut time.Time, u api.UsageDelta, input, cached int, stop, failure string, calls, images int) {
	ms := time.Since(start).Milliseconds()
	loop.stats.rounds++
	loop.stats.modelMs += ms
	if failure != "" {
		loop.stats.retries++
	}
	ttft := int64(-1)
	if !firstOut.IsZero() {
		ttft = firstOut.Sub(start).Milliseconds()
	}
	loop.Telemetry.Send("gordi_run", map[string]any{
		"session":          loop.Session.ID,
		"round":            loop.stats.rounds,
		"model":            loop.Config.Model,
		"mode":             loop.Session.ReasoningMode,
		"prompt_tokens":    input,
		"cached_tokens":    cached,
		"output_tokens":    u.OutputTokens,
		"reasoning_tokens": u.ReasoningTokens,
		"draft_tokens":     u.DraftTokens,
		"draft_accepted":   u.DraftAcceptedTokens,
		"ttft_ms":          ttft,
		"ms":               ms,
		"stop":             stop,
		"finish":           u.FinishReason,
		"failure":          failure,
		"calls":            calls,
		"images":           images,
	})
}

// emitTool records a call that ran (denied == "") or was refused: by a rule, or by you.
func (loop *ConversationLoop) emitTool(name string, ms int64, failed, trimmed, asked bool, denied string) {
	loop.stats.tools++
	loop.stats.toolMs += ms
	if failed {
		loop.stats.failed++
	}
	loop.Telemetry.Send("gordi_tool", map[string]any{
		"session": loop.Session.ID,
		"tool":    name,
		"ms":      ms,
		"failed":  failed,
		"trimmed": trimmed,
		"asked":   asked,
		"denied":  denied,
	})
}

// waitedOnYou adds time the turn spent on a permission box or an ask_user question.
func (loop *ConversationLoop) waitedOnYou(since time.Time) {
	loop.stats.youMs += time.Since(since).Milliseconds()
}

func (loop *ConversationLoop) emitTurn(outcome string) {
	s := loop.stats
	loop.Telemetry.Send("gordi_turn", map[string]any{
		"session":     loop.Session.ID,
		"source":      s.source,
		"outcome":     outcome,
		"rounds":      s.rounds,
		"tools":       s.tools,
		"failed":      s.failed,
		"retries":     s.retries,
		"compactions": s.compactions,
		"ms":          time.Since(s.start).Milliseconds(),
		"model_ms":    s.modelMs,
		"tool_ms":     s.toolMs,
		"you_ms":      s.youMs,
	})
}
