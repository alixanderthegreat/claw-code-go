package handoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, text string, at time.Time) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, at, at)
}

func doc(size, turns, body string) string {
	return "# Session Handoff: p\n**Date:** 2026-11-08\n**Original size:** " + size + ", 100 messages (" + turns + " operator turns, 50 API responses), `x`\n\n" + body
}

func TestProjectKey(t *testing.T) {
	if got := ProjectKey("/home/user/dev/simple-condense"); got != "-home-user-dev-simple-condense" {
		t.Errorf("got %q", got)
	}
	if got := ProjectKey("/tmp/a.b_c"); got != "-tmp-a-b-c" {
		t.Errorf("got %q", got)
	}
}

func TestLoad(t *testing.T) {
	dir, wd := t.TempDir(), "/work/proj"
	p := filepath.Join(dir, ProjectKey(wd))
	now := time.Now()
	write(t, filepath.Join(p, "b4a3e097-big", "handoff.md"), doc("209.6 MB", "1135", "the big one"), now.Add(-2*time.Minute))
	write(t, filepath.Join(p, "c1c00183-small", "handoff.md"), doc("1.4 MB", "10", "the small one"), now.Add(-time.Minute))
	write(t, filepath.Join(p, "latest-handoff.md"), doc("1.4 MB", "10", "the small one"), now.Add(-time.Minute))

	text, e, all, err := Load(dir, wd, "")
	if err != nil || !strings.Contains(text, "the small one") || e.ID != "c1c00183-small" || len(all) != 2 || all[0].ID != "c1c00183-small" {
		t.Fatalf("latest: %q %+v %d %v", text, e, len(all), err)
	}
	if text, e, _, err := Load(dir, wd, "b4a3"); err != nil || !strings.Contains(text, "the big one") || e.Turns != "1135" || e.Size != "209.6 MB" {
		t.Errorf("by prefix: %q %+v %v", text, e, err)
	}
	if _, _, _, err := Load(dir, wd, "zzz"); !errors.Is(err, ErrNone) {
		t.Errorf("an unknown prefix: %v", err)
	}
	if _, _, _, err := Load(dir, "/elsewhere", ""); !errors.Is(err, ErrNone) {
		t.Errorf("a folder with none: %v", err)
	}
	if got := Describe(all[1]); !strings.HasPrefix(got, "b4a3e097  ") || !strings.HasSuffix(got, "209.6 MB, 1135 turns") {
		t.Errorf("describe: %q", got)
	}
}

func TestDir(t *testing.T) {
	t.Setenv("CLAW_HANDOFF_DIR", "/x")
	if Dir() != "/x" {
		t.Error(Dir())
	}
	t.Setenv("CLAW_HANDOFF_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/data")
	if Dir() != "/data/simple-condense" {
		t.Error(Dir())
	}
}

func TestLocal(t *testing.T) {
	for u, want := range map[string]bool{
		"http://127.0.0.1:8080/v1": true, "http://localhost:1/v1": true, "http://192.168.1.5:8080/v1": true,
		"": false, "https://8.8.8.8/v1": false, "not a url": false,
	} {
		if got := Local(u); got != want {
			t.Errorf("Local(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestFindAnywhere(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write(t, filepath.Join(dir, "-a", "b4a3e097-big", "handoff.md"), doc("209.6 MB", "1135", "big"), now)
	write(t, filepath.Join(dir, "-a", "latest-handoff.md"), doc("209.6 MB", "1135", "big"), now)
	write(t, filepath.Join(dir, "-b", "c1c00183-small", "handoff.md"), doc("1.4 MB", "10", "small"), now.Add(-time.Hour))
	write(t, filepath.Join(dir, "-b", "latest-handoff.md"), doc("1.4 MB", "10", "small"), now.Add(-time.Hour))
	got, err := Find(dir, "b4a3")
	if err != nil || len(got) != 1 || got[0].ID != "b4a3e097-big" || Folder(got[0]) != "-a" {
		t.Fatalf("find: %+v %v", got, err)
	}
	if got, _ := Find(dir, "zz"); len(got) != 0 {
		t.Errorf("an unknown prefix found %+v", got)
	}
	if p := Projects(dir); len(p) != 2 || p[0] != "-a" {
		t.Errorf("projects %v: want -a (newest) then -b", p)
	}
}
