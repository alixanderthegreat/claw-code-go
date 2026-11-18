//go:build unix

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRTK puts an rtk on PATH that answers `rtk rewrite CMD` with script, run by sh with the
// command as $2.
func fakeRTK(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	body := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rtk"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestCompressCommand: rtk's version is taken on its exit codes 0 and 3, and the command runs as
// given on 1 (no version), 2 (rtk's own deny rule) or an empty answer.
func TestCompressCommand(t *testing.T) {
	for _, c := range []struct {
		name, script string
		want         string
		ok           bool
	}{
		{"allow rule", `echo "rtk $2"; exit 0`, "rtk git status", true},
		{"no rule", `echo "rtk $2"; exit 3`, "rtk git status", true},
		{"no version", `exit 1`, "git status", false},
		{"deny rule", `exit 2`, "git status", false},
		{"empty answer", `exit 0`, "git status", false},
		{"same command", `echo "$2"; exit 3`, "git status", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeRTK(t, c.script)
			got, ok := CompressCommand("git status")
			if got != c.want || ok != c.ok {
				t.Errorf("got %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

// TestCompressCommandKeepsLines: a command over several lines comes back whole, only rtk's own
// trailing newline dropped.
func TestCompressCommandKeepsLines(t *testing.T) {
	fakeRTK(t, `printf 'cd src\nrtk go test ./...\n'; exit 3`)
	got, ok := CompressCommand("cd src\ngo test ./...")
	if !ok || got != "cd src\nrtk go test ./..." {
		t.Errorf("got %q, %v", got, ok)
	}
}

// TestCompressCommandWithoutRTK: no rtk on PATH runs the command as given.
func TestCompressCommandWithoutRTK(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got, ok := CompressCommand("git status"); ok || got != "git status" {
		t.Errorf("got %q, %v; want it unchanged", got, ok)
	}
}

// TestBashKeepsNoRTKCopies: a command - through rtk or not - runs with rtk's recall store and
// telemetry off, so rtk keeps no second copy of what the model runs.
func TestBashKeepsNoRTKCopies(t *testing.T) {
	out, err := ExecuteBash(map[string]any{"command": "echo $RTK_RECALL $RTK_TELEMETRY_DISABLED"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "0 1" {
		t.Errorf("got %q, want RTK_RECALL=0 and RTK_TELEMETRY_DISABLED=1", out)
	}
}
