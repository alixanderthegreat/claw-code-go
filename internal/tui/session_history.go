package tui

import (
	"claw-code-go/internal/api"
	"encoding/json"
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
func renderHistory(messages []api.Message) string {
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
					toolLines = append(toolLines, style.Render(fmt.Sprintf("  %s %s → %s", mark, name, truncate(resultText, 40))))
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
					toolLines = append(toolLines, toolRunningStyle.Render(fmt.Sprintf("  ◆ %s: %s", block.Name, truncate(summarizeHistoryInput(block.Input), 60))))
				}
			}
			if len(texts) > 0 {
				sb.WriteString(assistantLabelStyle.Render("Gordi") + ": " + strings.Join(texts, "\n") + "\n\n")
			}
			for _, line := range toolLines {
				sb.WriteString(line + "\n")
			}
		}
	}

	return sb.String()
}

// summarizeHistoryInput renders a stored tool_use block's input compactly for the resumed-history
// view. Not the same helper as runtime's own summarizeToolInput (unexported in a different
// package) - this one just needs something readable, not the live permission-prompt's exact format.
func summarizeHistoryInput(input map[string]any) string {
	b, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	return string(b)
}
