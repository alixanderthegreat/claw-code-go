package tui

import (
	"claw-code-go/internal/api"
	"claw-code-go/internal/runtime"
	"fmt"
	"strings"
)

// renderHistory reconstructs a resumed session's past turns into the same visual shape the live
// conversation builds one piece at a time (see startMessage/streamDoneMsg in model.go) - without
// this, --session only restores the model's own context (loop.Session.Messages), and the TUI just
// shows the startup logo with no visible sign any history is actually there.
//
// "Thinking" content is never persisted to session messages in the first place (see
// runtime.runOneTurnStreaming's own comment on why), so there's nothing to reconstruct for it here.
func renderHistory(messages []api.Message, width int) string {
	if len(messages) == 0 {
		return ""
	}

	var sb strings.Builder
	toolNames := map[string]string{} // tool_use id -> tool name, so its later tool_result can name it

	for _, msg := range messages {
		var texts []string
		var toolLines []string

		switch msg.Role {
		case "user":
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						texts = append(texts, block.Text)
					}
				case "tool_result":
					name := toolNames[block.ToolUseID]
					if name == "" {
						name = "tool"
					}
					resultText := ""
					if len(block.Content) > 0 {
						resultText = block.Content[0].Text
					}
					style, mark := toolDoneStyle, "✓"
					if block.IsError {
						style, mark = toolFailedStyle, "✗"
					}
					if header, body, ok := consultDisplay(resultText, width); name == "consult" && ok && !block.IsError {
						// As live: a consultant's answer is there to be read, all of it.
						toolLines = append(toolLines, style.Render("  ✓ consult → "+header)+"\n"+body)
						continue
					}
					toolLines = append(toolLines, style.Render(fmt.Sprintf("  %s %s → %s", mark, name, cut(resultText, 500))))
				}
			}
			if len(texts) > 0 {
				sb.WriteString(userLabelStyle.Render("You") + ": " + strings.Join(texts, "\n") + "\n\n")
			}
			for _, line := range toolLines {
				sb.WriteString(line + "\n")
			}

		case "assistant":
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					if block.Text != "" {
						texts = append(texts, block.Text)
					}
				case "tool_use":
					toolNames[block.ID] = block.Name
					toolLines = append(toolLines, toolRunningStyle.Render(fmt.Sprintf("  ◆ %s: %s", block.Name, cut(runtime.SummarizeToolInput(block.Input), 300))))
				}
			}
			if len(texts) > 0 {
				sb.WriteString(assistantLabelStyle.Render("Gordi") + ": " + renderMarkdown(strings.Join(texts, "\n"), width) + "\n\n")
			}
			for _, line := range toolLines {
				sb.WriteString(line + "\n")
			}
		}
	}

	return sb.String()
}
