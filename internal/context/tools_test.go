package context

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// gordiMD is a machine GORDI.md in the shape simpleproject's CLAUDE.md lists its tools.
const gordiMD = "# This machine\n\n" +
	"A machine's own notes.\n\n" +
	"- `notatool` - a bullet before the Tools section isn't a tool\n\n" +
	"## Tools on PATH\n\n" +
	"- `library books | search \"q\" [-in books|notes|all]` - the book library. `library notes [waiting]` and\n" +
	"  `library note NAME` - the notes.\n" +
	"- `consult MODE \"question\" [-material FILE]` - ask one of the model's modes once.\n" +
	"- A transcript to consult on: `transcribe text ID -plain > FILE`, read it, then consult.\n" +
	"- `speak \"TEXT\"` (or `echo TEXT | speak`) - says it aloud.\n" +
	"  - `nested` - an indented bullet is part of the one above, not a tool\n" +
	"- `gopass` - a password store.\n" +
	"* `screenshot [-w | -r]` - a screenshot of the whole screen.\n\n" +
	"## Working here\n\n" +
	"- `go vet ./...` before calling something done - not a tool: another section\n"

func TestToolNames(t *testing.T) {
	want := []string{"library", "consult", "speak", "gopass", "screenshot"}
	if got := ToolNames(gordiMD); !reflect.DeepEqual(got, want) {
		t.Errorf("ToolNames = %q, want %q", got, want)
	}
	if got := ToolNames("# No tools here\n\n- `ls` - not under a Tools heading\n"); len(got) != 0 {
		t.Errorf("ToolNames with no Tools section = %q, want none", got)
	}
	if got := ToolNames("### Tools\n- `deep` - a Tools heading at any level counts\n"); !reflect.DeepEqual(got, []string{"deep"}) {
		t.Errorf("ToolNames under ### Tools = %q, want [deep]", got)
	}
}

// fakeTools puts each name on a fresh PATH as an executable, and nothing else.
func fakeTools(t *testing.T, names ...string) {
	t.Helper()
	bin := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
}

// TestToolsGate: the machine's GORDI.md is read into the instructions, and the tools it lists
// that aren't on PATH are named to the model as missing.
func TestToolsGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAW_MEMORY_DIR", filepath.Join(home, "mem"))
	write(t, MachineFile(), gordiMD)
	fakeTools(t, "library", "consult", "gopass")

	tools := Tools()
	if len(tools) != 5 || !tools[0].OnPath || tools[2].OnPath || tools[2].Name != "speak" {
		t.Fatalf("Tools = %+v, want 5 with speak not on PATH", tools)
	}
	s := Stable(home)
	if !strings.Contains(s, "A machine's own notes") {
		t.Errorf("the machine's GORDI.md isn't in the instructions:\n%s", s)
	}
	if !strings.Contains(s, "Not on this machine: `speak`, `screenshot`.") {
		t.Errorf("missing tools aren't named:\n%s", s)
	}

	fakeTools(t, "library", "consult", "speak", "gopass", "screenshot")
	if s := Stable(home); strings.Contains(s, "Not on this machine") {
		t.Errorf("with every tool on PATH, none should be named missing:\n%s", s)
	}
}

// TestGordiMDOrder: the machine's GORDI.md comes after the global CLAUDE.md, and a project's
// GORDI.md after the CLAUDE.md beside it - Gordi's word last in each place.
func TestGordiMDOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAW_MEMORY_DIR", filepath.Join(home, "mem"))
	proj := filepath.Join(home, "dev", "proj")
	write(t, filepath.Join(home, ".claude", "CLAUDE.md"), "GLOBAL")
	write(t, MachineFile(), "MACHINE")
	write(t, filepath.Join(proj, "CLAUDE.md"), "PROJECT-CLAUDE")
	write(t, filepath.Join(proj, "GORDI.md"), "PROJECT-GORDI")

	s := Stable(proj)
	order := []string{"GLOBAL", "MACHINE", "PROJECT-CLAUDE", "PROJECT-GORDI"}
	last := -1
	for _, w := range order {
		i := strings.Index(s, w)
		if i <= last {
			t.Fatalf("%s missing or out of order (want %v):\n%s", w, order, s)
		}
		last = i
	}
	if n := strings.Count(s, "MACHINE"); n != 1 {
		t.Errorf("the machine's GORDI.md read %d times, want once", n)
	}
}
