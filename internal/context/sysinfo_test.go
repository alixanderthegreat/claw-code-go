package context

import (
	"path/filepath"
	"strings"
	"testing"
)

// The model copies cwd into tool calls, and file tools don't expand ~ or resolve a relative path.
func TestSystemInfoAbsoluteCwd(t *testing.T) {
	got := SystemInfo(".")
	abs, _ := filepath.Abs(".")
	if !strings.HasPrefix(got, "cwd: "+abs+" | ") {
		t.Errorf("SystemInfo(.) = %q, want it to start with cwd: %s | ", got, abs)
	}
}

// Platform sits in the cached system prompt: it must not change between calls.
func TestPlatformStable(t *testing.T) {
	if a, b := Platform(), Platform(); a != b || !strings.HasPrefix(a, "OS: ") {
		t.Errorf("Platform() = %q then %q", a, b)
	}
}
