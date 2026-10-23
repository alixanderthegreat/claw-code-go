package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skill(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
}

// TestStableContext: instructions from home down to the working directory, nearest last; AGENTS.md
// beside CLAUDE.md; skills from ~/.claude/skills and every .claude/skills on the way down, a nearer
// one replacing a same-named one; the memory index.
func TestStableContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAW_MEMORY_DIR", filepath.Join(home, ".claw-code", "memory"))
	proj := filepath.Join(home, "dev", "proj")
	work := filepath.Join(proj, "cmd", "calendar")
	os.MkdirAll(work, 0o755)

	write(t, filepath.Join(home, ".claude", "CLAUDE.md"), "GLOBAL: be brief.")
	write(t, filepath.Join(proj, "CLAUDE.md"), "PROJECT: Gitea remotes over HTTP.")
	write(t, filepath.Join(work, "AGENTS.md"), "CALENDAR: never commit the binary.")
	write(t, filepath.Join(home, ".claude", "skills", "kata", "SKILL.md"), skill("kata", "run a kata cycle"))
	write(t, filepath.Join(home, ".claude", "skills", "deploy", "SKILL.md"), skill("deploy", "the old way"))
	write(t, filepath.Join(proj, ".claude", "skills", "deploy", "SKILL.md"), skill("deploy", "ship with deploy.sh"))
	write(t, filepath.Join(home, ".claude", "skills", "nodesc", "SKILL.md"), "no frontmatter at all")
	write(t, filepath.Join(home, ".claw-code", "memory", "MEMORY.md"), "- [Voice-first](voice.md) - speaks to Gordi")

	s := Stable(work)
	g, p, c := strings.Index(s, "GLOBAL"), strings.Index(s, "PROJECT"), strings.Index(s, "CALENDAR")
	if g < 0 || p < 0 || c < 0 || !(g < p && p < c) {
		t.Errorf("instructions missing or out of order (global %d, project %d, calendar %d):\n%s", g, p, c, s)
	}
	if !strings.Contains(s, "- deploy: ship with deploy.sh (~/dev/proj/.claude/skills/deploy/SKILL.md)") || strings.Contains(s, "the old way") {
		t.Errorf("the project's deploy skill should replace the global one:\n%s", s)
	}
	if !strings.Contains(s, "- kata: run a kata cycle") || strings.Contains(s, "nodesc") {
		t.Errorf("skills index wrong:\n%s", s)
	}
	if !strings.Contains(s, "- [Voice-first](voice.md) - speaks to Gordi") {
		t.Errorf("memory index missing:\n%s", s)
	}
}

// TestStableCachedUntilAFileChanges: the block holds still - so the system prompt and its cache do -
// until one of its files changes.
func TestStableCachedUntilAFileChanges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAW_MEMORY_DIR", filepath.Join(home, "mem"))
	write(t, filepath.Join(home, "CLAUDE.md"), "first")
	a := NewAssembler(home)
	if !strings.Contains(a.Stable(), "first") || a.Stable() != a.Stable() {
		t.Fatal("stable block not stable")
	}
	time.Sleep(10 * time.Millisecond)
	write(t, filepath.Join(home, "CLAUDE.md"), "second")
	if !strings.Contains(a.Stable(), "second") {
		t.Errorf("an edited CLAUDE.md wasn't picked up")
	}
	if strings.Contains(a.Assemble(), "second") {
		t.Errorf("instructions still ride in the per-message block")
	}
}
