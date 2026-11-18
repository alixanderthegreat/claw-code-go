package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenMCPLog: the log appends, is yours alone, and starts again once it's past maxMCPLog.
func TestOpenMCPLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "mcp.log")
	for _, line := range []string{"one\n", "two\n"} {
		f, err := openMCPLog(path)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(line)
		f.Close()
	}
	if b, _ := os.ReadFile(path); string(b) != "one\ntwo\n" {
		t.Errorf("got %q, want both lines", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}

	if err := os.Truncate(path, maxMCPLog+1); err != nil {
		t.Fatal(err)
	}
	f, err := openMCPLog(path)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("fresh\n")
	f.Close()
	if b, _ := os.ReadFile(path); string(b) != "fresh\n" {
		t.Errorf("past the cap: got %d bytes, want it started afresh", len(b))
	}
}
