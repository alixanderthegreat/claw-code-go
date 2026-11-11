package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTildeIsHome: "~/..." in a file tool's path is the home directory, not a folder named "~"
// in the working directory (greatproject #439).
func TestTildeIsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wd := t.TempDir()
	t.Chdir(wd)

	if _, err := ExecuteWriteFile(map[string]any{"path": "~/tasks/a.md", "content": "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "tasks", "a.md")); err != nil {
		t.Errorf("not written under home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wd, "~")); err == nil {
		t.Errorf("a folder named ~ was made in the working directory")
	}
	if got, err := ExecuteReadFile(map[string]any{"path": "~/tasks/a.md"}); err != nil || !strings.Contains(got, "hello") {
		t.Errorf("read_file ~/tasks/a.md = %q, %v", got, err)
	}
	if _, err := ExecuteFileEdit(map[string]any{"file_path": "~/tasks/a.md", "old_string": "hello", "new_string": "bye"}); err != nil {
		t.Errorf("file_edit: %v", err)
	}
	if got, err := ExecuteGlob(map[string]any{"pattern": "*.md", "path": "~/tasks"}); err != nil || !strings.Contains(got, "a.md") {
		t.Errorf("glob in ~/tasks = %q, %v", got, err)
	}
	if got, err := ExecuteGrep(map[string]any{"pattern": "bye", "path": "~/tasks"}); err != nil || !strings.Contains(got, "a.md") {
		t.Errorf("grep in ~/tasks = %q, %v", got, err)
	}
	for in, want := range map[string]string{"~": home, "~/x": filepath.Join(home, "x"), "~bob/x": "~bob/x", "a/~/b": "a/~/b"} {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}
