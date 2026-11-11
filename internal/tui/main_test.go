package tui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps the TUI's saved choices (tui.json) out of the tests: a panel the operator put
// away mustn't change what a test sees, and a test's toggle mustn't land in their config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tui-prefs")
	if err != nil {
		panic(err)
	}
	prefsPath = func() string { return filepath.Join(dir, "tui.json") }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
