package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.go")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFileEditReplaceAll: a string that appears more than once can be changed everywhere at once,
// and without replace_all the error says how to proceed.
func TestFileEditReplaceAll(t *testing.T) {
	path := writeTemp(t, "a := old\nb := old\n")
	_, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "old", "new_string": "new"})
	if err == nil || !strings.Contains(err.Error(), "replace_all") {
		t.Fatalf("ambiguous match error should mention replace_all, got %v", err)
	}
	out, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "old", "new_string": "new", "replace_all": true})
	if err != nil || !strings.Contains(out, "2 occurrences") {
		t.Fatalf("replace_all = %q, %v", out, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "a := new\nb := new\n" {
		t.Errorf("file is %q", b)
	}
}

// TestFileEditWhitespaceHint: the commonest miss - right text, wrong indentation - says so.
func TestFileEditWhitespaceHint(t *testing.T) {
	path := writeTemp(t, "func f() {\n\treturn 1\n}\n")
	_, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "    return 1\n", "new_string": "\treturn 2\n"})
	if err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Errorf("want a whitespace hint, got %v", err)
	}
}

// TestWriteFileOverExistingSteersToEdit: replacing an existing file still works, and the result
// points at file_edit for next time; a new file says nothing of the kind.
func TestWriteFileOverExistingSteersToEdit(t *testing.T) {
	path := writeTemp(t, "old\n")
	out, err := ExecuteWriteFile(map[string]any{"path": path, "content": "new\n"})
	if err != nil || !strings.Contains(out, "file_edit") {
		t.Fatalf("overwrite = %q, %v", out, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new\n" {
		t.Errorf("file is %q", b)
	}
	fresh := filepath.Join(t.TempDir(), "new.go")
	out, err = ExecuteWriteFile(map[string]any{"path": fresh, "content": "x"})
	if err != nil || strings.Contains(out, "file_edit") {
		t.Errorf("new file = %q, %v", out, err)
	}
}
