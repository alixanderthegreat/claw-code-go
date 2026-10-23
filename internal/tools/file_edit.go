package tools

import (
	"claw-code-go/internal/api"
	"fmt"
	"os"
	"strings"
)

// FileEditTool returns the tool definition for targeted string-replace file edits.
func FileEditTool() api.Tool {
	return api.Tool{
		Name:        "file_edit",
		Description: "The way to change an existing file: replace an exact string in it with new content, sending only the part that changes. Copy old_string exactly from read_file's output, whitespace and indentation included, with enough surrounding lines to match just once - or set replace_all to change every occurrence. Several changes to one file are several file_edit calls; make them together in one response. You must read the file with read_file in this session before editing it, so the edit is grounded in its real, current content.",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"file_path": {
					Type:        "string",
					Description: "Path to the file to edit",
				},
				"old_string": {
					Type:        "string",
					Description: "Exact string to find and replace (must appear exactly once)",
				},
				"new_string": {
					Type:        "string",
					Description: "Replacement string",
				},
				"replace_all": {
					Type:        "boolean",
					Description: "Replace every occurrence of old_string instead of requiring exactly one (default false)",
				},
			},
			Required: []string{"file_path", "old_string", "new_string"},
		},
	}
}

// ExecuteFileEdit performs a targeted string replacement in a file.
func ExecuteFileEdit(input map[string]any) (string, error) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return "", fmt.Errorf("file_edit: 'file_path' is required")
	}
	if err := requireWithinDispatchBoundary(filePath); err != nil {
		return "", fmt.Errorf("file_edit: %w", err)
	}
	oldString, ok := input["old_string"].(string)
	if !ok {
		return "", fmt.Errorf("file_edit: 'old_string' is required")
	}
	newString, ok := input["new_string"].(string)
	if !ok {
		return "", fmt.Errorf("file_edit: 'new_string' is required")
	}
	// A real, live transcript on 2026-09-22 showed the model report a no-op edit as a
	// success, then spend a whole extra turn re-reading the file and reasoning out on
	// its own that "old_string and new_string are identical" before it could retry with
	// an actual change. Catching this here means the model finds out immediately, in the
	// same tool call, instead of discovering it after the fact.
	if oldString == newString {
		return "", fmt.Errorf("file_edit: old_string and new_string are identical - no change would be made; if you meant to change something, old_string does not match what you intended")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("file_edit: %w", err)
	}

	content := string(data)
	replaceAll, _ := input["replace_all"].(bool)
	count := strings.Count(content, oldString)
	if count == 0 {
		hint := "copy it exactly from read_file's output, whitespace and indentation included"
		if strings.Contains(content, strings.TrimSpace(oldString)) {
			hint = "it does appear with different leading/trailing whitespace - copy the exact indentation and line breaks"
		}
		return "", fmt.Errorf("file_edit: old_string not found in %s: %s; if the file changed since you read it, read_file it again", filePath, hint)
	}
	if count > 1 && !replaceAll {
		return "", fmt.Errorf("file_edit: old_string matches %d locations in %s - add surrounding lines to make it unique, or set replace_all to change all %d", count, filePath, count)
	}

	n := 1
	if replaceAll {
		n = -1
	}
	updated := strings.Replace(content, oldString, newString, n)
	if err := os.WriteFile(filePath, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("file_edit: write: %w", err)
	}

	if replaceAll && count > 1 {
		return fmt.Sprintf("Successfully edited %s (%d occurrences)", filePath, count), nil
	}
	return fmt.Sprintf("Successfully edited %s", filePath), nil
}
