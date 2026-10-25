package tools

import (
	"bufio"
	"bytes"
	"claw-code-go/internal/api"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxGrepResults = 1000

// maxGrepLine bounds one matching line in the output: a minified file's single line is a match.
const maxGrepLine = 500

// errBinary marks a file grep skips, as grep -I does: an MP3 in a searched folder used to come
// back as "matches" of raw bytes, which the model can't use and which garbled the TUI.
var errBinary = errors.New("binary file")

// GrepTool returns the tool definition for grep file search.
func GrepTool() api.Tool {
	return api.Tool{
		Name:        "grep",
		Description: "Search for a regex pattern in files. Returns matching lines in file:line:content format.",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"pattern": {
					Type:        "string",
					Description: "Regular expression pattern to search for",
				},
				"path": {
					Type:        "string",
					Description: "Directory or file to search in",
				},
				"glob": {
					Type:        "string",
					Description: "File glob filter (e.g., '*.go', '*.ts'). Only used when path is a directory.",
				},
			},
			Required: []string{"pattern", "path"},
		},
	}
}

// ExecuteGrep searches for a regex pattern in files.
func ExecuteGrep(input map[string]any) (string, error) {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("grep: 'pattern' input is required and must be a string")
	}

	searchPath, ok := input["path"].(string)
	if !ok || searchPath == "" {
		return "", fmt.Errorf("grep: 'path' input is required and must be a string")
	}

	globFilter := ""
	if g, ok := input["glob"].(string); ok {
		globFilter = g
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("grep: invalid pattern: %w", err)
	}

	var results []string
	binaries := 0

	info, err := os.Stat(searchPath)
	if err != nil {
		return "", fmt.Errorf("grep: stat path: %w", err)
	}

	if info.IsDir() {
		err = filepath.Walk(searchPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				// Skip hidden directories
				if strings.HasPrefix(fi.Name(), ".") && path != searchPath {
					return filepath.SkipDir
				}
				return nil
			}

			// Apply glob filter if specified
			if globFilter != "" {
				matched, err := filepath.Match(globFilter, filepath.Base(path))
				if err != nil || !matched {
					return nil
				}
			}

			fileResults, err := grepFile(re, path)
			if err == errBinary {
				binaries++
				return nil
			}
			if err != nil {
				return nil // Skip unreadable files
			}
			results = append(results, fileResults...)

			if len(results) >= maxGrepResults {
				return filepath.SkipAll
			}

			return nil
		})
		if err != nil && err != filepath.SkipAll {
			return "", fmt.Errorf("grep walk: %w", err)
		}
	} else {
		fileResults, err := grepFile(re, searchPath)
		if err == errBinary {
			return "", fmt.Errorf("grep: %s is a binary file", searchPath)
		}
		if err != nil {
			return "", fmt.Errorf("grep: %w", err)
		}
		results = fileResults
	}

	skipped := ""
	if binaries > 0 {
		skipped = fmt.Sprintf("(%d binary files skipped)", binaries)
	}
	if len(results) == 0 {
		return strings.TrimSpace(fmt.Sprintf("No matches found for pattern: %s %s", pattern, skipped)), nil
	}

	output := strings.Join(results, "\n")
	if len(results) >= maxGrepResults {
		output += fmt.Sprintf("\n... [truncated at %d results]", maxGrepResults)
	}
	if skipped != "" {
		output += "\n" + skipped
	}

	return output, nil
}

// grepFile searches a single file for the regex pattern.
func grepFile(re *regexp.Regexp, path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head := make([]byte, 8192)
	n, _ := f.Read(head)
	if isBinary(head[:n]) {
		return nil, errBinary
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	var results []string
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			if len(line) > maxGrepLine {
				cut := maxGrepLine
				for cut > 0 && !utf8.RuneStart(line[cut]) {
					cut--
				}
				line = line[:cut] + fmt.Sprintf("... [%d more bytes]", len(line)-cut)
			}
			results = append(results, fmt.Sprintf("%s:%d:%s", path, lineNum, line))
		}
	}

	return results, scanner.Err()
}

// isBinary reports whether a file's first bytes are binary: a NUL, or not UTF-8 (allowing for a
// character cut off at the end of the sample).
func isBinary(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}
	for i := 0; i < 4 && len(head) > 0; i++ {
		if utf8.Valid(head) {
			return false
		}
		head = head[:len(head)-1]
	}
	return !utf8.Valid(head)
}
