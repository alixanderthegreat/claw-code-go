package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPruneKeepsFewAndRecent: shots past keepFor go, and so does all but the newest keepLast;
// clean takes them all.
func TestPruneKeepsFewAndRecent(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	ages := []time.Duration{0, time.Minute, 2 * time.Minute, 3 * time.Minute, 20 * time.Minute}
	for i, age := range ages {
		p := filepath.Join(dir, "shot-"+string(rune('a'+i))+".png")
		os.WriteFile(p, []byte("x"), 0o600)
		os.Chtimes(p, now.Add(-age), now.Add(-age))
	}
	if n := prune(dir, keepFor, keepLast, now); n != 2 {
		t.Errorf("pruned %d, want 2: the oldest by count and the one past 15 minutes", n)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "shot-*.png"))
	if len(left) != keepLast {
		t.Errorf("%d left, want %d", len(left), keepLast)
	}
	if n := prune(dir, 0, 0, now); n != keepLast {
		t.Errorf("clean deleted %d, want %d", n, keepLast)
	}
}

func TestShotsDirIsPrivate(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	dir, err := shotsDir()
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Errorf("mode %v, want 0700", info.Mode().Perm())
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	if _, err := shotsDir(); err == nil {
		t.Error("without XDG_RUNTIME_DIR it should refuse, not write to disk")
	}
}
