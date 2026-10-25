package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	clawctx "claw-code-go/internal/context"
)

// card.go: what the model is shown - its cue card - part by part, so an answer can be traced to
// what it had: which instructions and memories were loaded, how big each part was, whether a
// compaction summary stood in for history, which mode. Two views:
//
//	gordi_card  one telemetry event per turn: each part's size and a fingerprint of the system
//	            prompt - names and numbers only, like every gordi_* event. A fingerprint that
//	            changes mid-session means kronk reads the whole prompt fresh on that turn.
//	/card       the same table on screen, with the system prompt and summary in full. That
//	            content stays on device.

// CardPart is one part of what the model is sent.
type CardPart struct {
	Name   string
	Tokens int    // estimated, at charsPerToken
	Detail string // what it's made of: files, counts
}

// cardView is the card, measured.
type cardView struct {
	parts       []CardPart
	sections    clawctx.Sections
	system      string
	toolsTokens int
	historyMsgs int
	images      int
	hash        string
}

func (loop *ConversationLoop) cardView() cardView {
	var v cardView
	v.system = loop.systemPrompt()
	toolsJSON, _ := json.Marshal(loop.allTools())
	v.toolsTokens = len(toolsJSON) / charsPerToken
	sum := sha256.Sum256(append([]byte(v.system), toolsJSON...))
	v.hash = hex.EncodeToString(sum[:4])

	base := systemPromptBase
	if !loop.modeReplacesIdentity() {
		base = systemPromptIdentity + " " + systemPromptBase
	}
	if loop.CtxAssembler != nil {
		v.sections = clawctx.StableSections(loop.CtxAssembler.WorkDir)
	}
	v.historyMsgs = len(loop.Session.Messages)
	v.images = countImages(loop.Session.Messages)
	mode := loop.Session.ReasoningMode
	if mode == "" {
		mode = "default"
	}
	summary := loop.Session.CompactionSummary
	v.parts = []CardPart{
		{"base rules", len(base) / charsPerToken, "who Gordi is and how to use the tools"},
		{"instructions", len(v.sections.Instructions) / charsPerToken, plural(len(v.sections.InstructionFiles), "file") + filesDetail(v.sections.InstructionFiles)},
		{"skills", len(v.sections.Skills) / charsPerToken, plural(v.sections.SkillCount, "skill")},
		{"memory index", len(v.sections.Memory) / charsPerToken, plural(v.sections.MemoryLines, "memory")},
		{"tool schemas", v.toolsTokens, plural(len(loop.allTools()), "tool")},
		{"compaction summary", len(summary) / charsPerToken, plural(loop.Session.CompactionCount, "compaction") + " this session"},
		{"history", EstimateTokens(loop.Session.Messages), plural(v.historyMsgs, "message") + imagesDetail(v.images)},
		{"mode", 0, mode},
	}
	return v
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	if strings.HasSuffix(what, "y") {
		return fmt.Sprintf("%d %sies", n, what[:len(what)-1])
	}
	return fmt.Sprintf("%d %ss", n, what)
}

func filesDetail(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return ": " + strings.Join(files, ", ")
}

func imagesDetail(n int) string {
	if n == 0 {
		return ""
	}
	return ", " + plural(n, "image")
}

// emitCard sends this turn's gordi_card, and notes whether the system prompt changed since the
// last turn - the cache breaker the blueprint calls out: an edited CLAUDE.md or memory mid-session.
func (loop *ConversationLoop) emitCard() {
	if loop.Telemetry == nil {
		return
	}
	v := loop.cardView()
	changed := loop.lastCardHash != "" && loop.lastCardHash != v.hash
	loop.lastCardHash = v.hash
	mode := loop.Session.ReasoningMode
	loop.Telemetry.Send("gordi_card", map[string]any{
		"session":             loop.Session.ID,
		"system_hash":         v.hash,
		"system_changed":      changed,
		"mode":                mode,
		"base_tokens":         v.parts[0].Tokens,
		"instructions_tokens": v.parts[1].Tokens,
		"instruction_files":   len(v.sections.InstructionFiles),
		"skills":              v.sections.SkillCount,
		"skills_tokens":       v.parts[2].Tokens,
		"memories":            v.sections.MemoryLines,
		"memory_tokens":       v.parts[3].Tokens,
		"tools_tokens":        v.toolsTokens,
		"summary_tokens":      v.parts[5].Tokens,
		"compactions":         loop.Session.CompactionCount,
		"history_messages":    v.historyMsgs,
		"history_tokens":      v.parts[6].Tokens,
		"images":              v.images,
	})
}

// CardText is /card: the parts and their sizes, then the system prompt and the compaction summary
// in full - what the next request will carry ahead of your message.
func (loop *ConversationLoop) CardText() string {
	v := loop.cardView()
	var b strings.Builder
	b.WriteString("What the model is sent, ahead of your next message (tokens estimated at ~4 characters each):\n\n")
	total := 0
	for _, p := range v.parts {
		if p.Name == "mode" {
			fmt.Fprintf(&b, "  %-19s %8s  %s\n", p.Name, "", p.Detail)
			continue
		}
		total += p.Tokens
		fmt.Fprintf(&b, "  %-19s %8d  %s\n", p.Name, p.Tokens, p.Detail)
	}
	fmt.Fprintf(&b, "  %-19s %8d  of a %d-token window; compaction at ~%d\n", "total", total, loop.Config.ContextWindow, compactionThreshold(loop.Config))
	fmt.Fprintf(&b, "\nSystem fingerprint %s - it changes when instructions, skills, memory or the mode change, and the next request is then read fresh.\n", v.hash)
	b.WriteString("\n--- system prompt ---\n")
	b.WriteString(v.system)
	b.WriteString("\n")
	if s := loop.Session.CompactionSummary; s != "" {
		b.WriteString("\n--- compaction summary (stands in for the older history) ---\n")
		b.WriteString(s)
		b.WriteString("\n")
	}
	return b.String()
}
