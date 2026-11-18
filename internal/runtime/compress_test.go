//go:build unix

package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCompressInput: with compress_output on, a bash command goes through rtk's version while the
// model's own input stays as it gave it; off, or for another tool, nothing changes.
func TestCompressInput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rtk"), []byte("#!/bin/sh\necho \"rtk $2\"; exit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	given := map[string]any{"command": "git status", "timeout": float64(30)}

	off := &ConversationLoop{Config: &Config{}}
	if got, ok := off.compressInput("bash", given); ok || got["command"] != "git status" {
		t.Errorf("off: got %v, %v; want it unchanged", got["command"], ok)
	}

	on := &ConversationLoop{Config: &Config{CompressOutput: true}}
	if _, ok := on.compressInput("read_file", map[string]any{"path": "x"}); ok {
		t.Error("read_file went through rtk")
	}
	got, ok := on.compressInput("bash", given)
	if !ok || got["command"] != "rtk git status" || got["timeout"] != float64(30) {
		t.Errorf("on: got %v, %v; want rtk git status with the timeout kept", got, ok)
	}
	if given["command"] != "git status" {
		t.Errorf("the model's input changed to %q", given["command"])
	}
}
