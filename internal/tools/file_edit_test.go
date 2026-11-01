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

// TestFileEditWhitespaceHint: right text with a part of a line missing still refuses, and says
// the text is there with different whitespace when it is.
func TestFileEditWhitespaceHint(t *testing.T) {
	path := writeTemp(t, "func f() {\n\treturn 1 // one\n}\n")
	_, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "  return 1  ", "new_string": "return 2"})
	if err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Errorf("want a whitespace hint, got %v", err)
	}
}

// TestFileEditIgnoresIndentation: the commonest miss - whole lines, spaces where the file has
// tabs - is applied when it matches one place, and the new text takes the file's tabs.
func TestFileEditIgnoresIndentation(t *testing.T) {
	path := writeTemp(t, "func f() {\n\tif x {\n\t\treturn 1\n\t}\n}\n")
	out, err := ExecuteFileEdit(map[string]any{"file_path": path,
		"old_string": "    if x {\n        return 1\n    }\n",
		"new_string": "    if x {\n        return 2\n    }\n"})
	if err != nil || !strings.Contains(out, "ignoring") {
		t.Fatalf("got %q, %v", out, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "func f() {\n\tif x {\n\t\treturn 2\n\t}\n}\n" {
		t.Errorf("file is %q", b)
	}
}

// TestFileEditIgnoresTrailingSpace: a trailing space on old_string's line is no miss either.
func TestFileEditIgnoresTrailingSpace(t *testing.T) {
	path := writeTemp(t, "a := 1\nb := 2\n")
	if _, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "b := 2   ", "new_string": "b := 3"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "a := 1\nb := 3\n" {
		t.Errorf("file is %q", b)
	}
}

// TestFileEditLooseNeedsOnePlace: two places that match once whitespace is ignored refuse with the
// count, and the file is untouched; a line that differs in a word never loosely matches.
func TestFileEditLooseNeedsOnePlace(t *testing.T) {
	path := writeTemp(t, "\tx := 1\n\ty := 2\n\tx := 1\n")
	_, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "  x := 1", "new_string": "x := 9"})
	if err == nil || !strings.Contains(err.Error(), "2 places") {
		t.Fatalf("want the count, got %v", err)
	}
	if _, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "  x := 7", "new_string": "x := 9"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a different line must not match, got %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "\tx := 1\n\ty := 2\n\tx := 1\n" {
		t.Errorf("file changed: %q", b)
	}
}

// TestFileEditExactWins: when old_string matches exactly, the loose path never runs.
func TestFileEditExactWins(t *testing.T) {
	path := writeTemp(t, "  v := 1\n\tv := 1\n")
	out, err := ExecuteFileEdit(map[string]any{"file_path": path, "old_string": "\tv := 1", "new_string": "\tv := 2"})
	if err != nil || strings.Contains(out, "ignoring") {
		t.Fatalf("got %q, %v", out, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "  v := 1\n\tv := 2\n" {
		t.Errorf("file is %q", b)
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
