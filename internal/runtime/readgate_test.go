package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestLoop() *ConversationLoop {
	return NewConversationLoop(&Config{Model: "test-model", MaxTokens: 1024}, nil)
}

// TestWriteFileRefusedWithoutPriorRead guards the core safety rule this
// gate exists for: overwriting a file the model never actually looked at
// in this session must fail, so edits can't be grounded in a guess.
func TestWriteFileRefusedWithoutPriorRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	loop := newTestLoop()
	result := loop.ExecuteToolQuiet("write_file", map[string]any{"path": path, "content": "new"})
	if !result.IsError {
		t.Fatalf("expected write_file to be refused without a prior read_file, got success")
	}

	got, _ := os.ReadFile(path)
	if string(got) != "original\n" {
		t.Fatalf("file was modified despite the refusal: %q", got)
	}
}

// TestWriteFileAllowedAfterRead confirms the gate opens once the model has
// actually read the file in this session.
func TestWriteFileAllowedAfterRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	loop := newTestLoop()
	if r := loop.ExecuteToolQuiet("read_file", map[string]any{"path": path}); r.IsError {
		t.Fatalf("read_file failed: %v", r.Content)
	}

	result := loop.ExecuteToolQuiet("write_file", map[string]any{"path": path, "content": "new\n"})
	if result.IsError {
		t.Fatalf("write_file should be allowed after read_file, got error: %v", result.Content)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new\n" {
		t.Fatalf("expected file to be updated, got %q", got)
	}
}

// TestWriteFileNewFileNeedsNoPriorRead ensures the gate only applies to
// overwriting existing content — creating a brand-new file is exempt, since
// there's nothing to have read yet.
func TestWriteFileNewFileNeedsNoPriorRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brandnew.txt")

	loop := newTestLoop()
	result := loop.ExecuteToolQuiet("write_file", map[string]any{"path": path, "content": "hello\n"})
	if result.IsError {
		t.Fatalf("creating a new file should not require a prior read, got error: %v", result.Content)
	}
}

// TestFileEditRefusedWithoutPriorRead applies the same rule to file_edit.
func TestFileEditRefusedWithoutPriorRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	loop := newTestLoop()
	result := loop.ExecuteToolQuiet("file_edit", map[string]any{
		"file_path": path, "old_string": "original", "new_string": "changed",
	})
	if !result.IsError {
		t.Fatalf("expected file_edit to be refused without a prior read_file, got success")
	}
}
