package tools

import (
	"claw-code-go/internal/api"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxDirectReadBytes is the largest file read_file will return in full
// without an explicit offset/limit. Above this, the model must state a
// bounded range instead of pulling an unknown-sized file straight into
// context.
const maxDirectReadBytes = 256 * 1024 // 256 KB

// maxReadLines caps how many lines a single bounded read_file call can
// return, even if the caller asks for more.
const maxReadLines = 2000

// ReadFileTool returns the tool definition for reading files.
func ReadFileTool() api.Tool {
	return api.Tool{
		Name: "read_file",
		Description: fmt.Sprintf(
			"Read the contents of a file from the filesystem. Files over %s must be read with 'offset'/'limit' instead of all at once — check the size first (e.g. via bash) for anything you suspect is large.",
			formatBytes(maxDirectReadBytes),
		),
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"path": {
					Type:        "string",
					Description: "Path to the file to read",
				},
				"offset": {
					Type:        "integer",
					Description: "1-indexed line number to start reading from (required for files over the direct-read size limit)",
				},
				"limit": {
					Type:        "integer",
					Description: fmt.Sprintf("Maximum number of lines to read, capped at %d", maxReadLines),
				},
			},
			Required: []string{"path"},
		},
	}
}

// WriteFileTool returns the tool definition for writing files.
func WriteFileTool() api.Tool {
	return api.Tool{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist. To overwrite an existing file, you must have read it first with read_file in this session, so the change is grounded in its real content rather than a guess.",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"path": {
					Type:        "string",
					Description: "Path to the file to write",
				},
				"content": {
					Type:        "string",
					Description: "Content to write to the file",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

// ExecuteReadFile reads a file and returns its contents. Files over
// maxDirectReadBytes are refused unless the caller supplies 'offset' and/or
// 'limit' to read a bounded line range instead.
func ExecuteReadFile(input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("read_file: 'path' input is required and must be a string")
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}

	offset, hasOffset := intInput(input, "offset")
	limit, hasLimit := intInput(input, "limit")
	bounded := hasOffset || hasLimit

	if !bounded && info.Size() > maxDirectReadBytes {
		lineCount, _ := countLines(path)
		return "", fmt.Errorf(
			"read_file: %s is %s (%d lines), over the %s direct-read limit. Use grep to search within it, or call read_file again with 'offset' and 'limit' to read a bounded range (e.g. offset=1, limit=%d)",
			path, formatBytes(info.Size()), lineCount, formatBytes(maxDirectReadBytes), maxReadLines,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}

	if !bounded {
		return string(data), nil
	}

	lines := strings.Split(string(data), "\n")
	start := 0
	if hasOffset {
		start = offset - 1
		if start < 0 {
			start = 0
		}
	}
	if start > len(lines) {
		return "", fmt.Errorf("read_file: offset %d is past end of file (%d lines)", offset, len(lines))
	}

	count := maxReadLines
	if hasLimit && limit < count {
		count = limit
	}
	end := start + count
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n"), nil
}

// intInput extracts an integer from a tool-call input map. JSON numbers
// decode as float64, so both forms are accepted.
func intInput(input map[string]any, key string) (int, bool) {
	v, ok := input[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

// countLines counts newlines in a file without holding its full contents
// in memory at once, for reporting size on files too large to read directly.
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 64*1024)
	count := 0
	for {
		n, err := f.Read(buf)
		count += strings.Count(string(buf[:n]), "\n")
		if err != nil {
			break
		}
	}
	return count, nil
}

// formatBytes renders a byte count in human-readable KB/MB units.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n1 := n / unit; n1 >= unit; n1 /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ExecuteWriteFile writes content to a file, creating parent directories as needed.
func ExecuteWriteFile(input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("write_file: 'path' input is required and must be a string")
	}

	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("write_file: 'content' input is required and must be a string")
	}

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("write_file: create directories: %w", err)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}
