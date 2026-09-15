package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecuteReadFileRefusesLargeFileWithoutRange guards against a large file
// silently blowing out the model's context window: without offset/limit,
// read_file must refuse anything over maxDirectReadBytes rather than return
// the whole thing.
func TestExecuteReadFileRefusesLargeFileWithoutRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	line := strings.Repeat("x", 100) + "\n"
	var sb strings.Builder
	for sb.Len() <= maxDirectReadBytes {
		sb.WriteString(line)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := ExecuteReadFile(map[string]any{"path": path})
	if err == nil {
		t.Fatalf("expected refusal for a file over the direct-read limit, got success")
	}
	if !strings.Contains(err.Error(), "offset") {
		t.Fatalf("expected the refusal to mention 'offset' as the way forward, got: %v", err)
	}
}

// TestExecuteReadFileBoundedRangeBypassesSizeGate confirms the actual
// escape hatch works: offset/limit lets the model read a large file anyway,
// in a bounded slice.
func TestExecuteReadFileBoundedRangeBypassesSizeGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	var sb strings.Builder
	for i := 1; sb.Len() <= maxDirectReadBytes; i++ {
		sb.WriteString(strings.Repeat("x", 90))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out, err := ExecuteReadFile(map[string]any{"path": path, "offset": float64(1), "limit": float64(3)})
	if err != nil {
		t.Fatalf("bounded read should succeed on a large file, got: %v", err)
	}
	if got := strings.Count(out, "\n") + 1; got != 3 {
		t.Fatalf("expected 3 lines back, got %d", got)
	}
}

// TestExecuteReadFileSmallFileNeedsNoRange ensures the common case (small
// files) is unaffected: no offset/limit required, full content returned.
func TestExecuteReadFileSmallFileNeedsNoRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "small.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out, err := ExecuteReadFile(map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error reading a small file: %v", err)
	}
	if out != "hello\n" {
		t.Fatalf("expected full content back, got %q", out)
	}
}
