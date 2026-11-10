package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const wideTable = "All three:\n\n| Book | Passages | Status |\n|---|---|---|\n" +
	"| Enterprise AI For Dummies — Zachary Jarvinen | 889 | indexing |\n" +
	"| How I Rob Banks — FC (FreakyClown) | 507 | readable, not yet in the list of books that are searchable today |\n"

func TestWideTableFits(t *testing.T) {
	for _, w := range []int{50, 60, 80} {
		out := ansi.Strip(renderMarkdown(wideTable, w))
		lines := strings.Split(out, "\n")
		cross, rules := -1, 0
		for _, l := range lines {
			if ansi.StringWidth(l) > w-2 {
				t.Errorf("width %d: line %d wide: %q", w, ansi.StringWidth(l), l)
			}
			if isTableRule(l) {
				rules++
				cross = ansi.StringWidth(l[:strings.Index(l, "┼")])
			} else if strings.TrimSpace(l) != "" && strings.Trim(strings.TrimSpace(l), "─") == "" {
				t.Errorf("width %d: a stray rule fragment: %q", w, l)
			}
		}
		if rules != 1 {
			t.Fatalf("width %d: %d rules in\n%s", w, rules, out)
		}
		// Every row's first bar sits over the rule's first cross.
		for _, l := range lines {
			if i := strings.Index(l, "│"); i >= 0 && ansi.StringWidth(l[:i]) != cross {
				t.Errorf("width %d: bar out of line: %q", w, l)
			}
		}
		for _, word := range []string{"Jarvinen", "889", "507", "searchable", "today"} {
			if !strings.Contains(out, word) {
				t.Errorf("width %d: %q lost", w, word)
			}
		}
	}
}

func TestNarrowTableUntouched(t *testing.T) {
	md := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	if got, want := renderMarkdown(md, 80), renderMarkdown(md, 200); got != want {
		t.Errorf("a table that fits changed:\n%q\n%q", got, want)
	}
}

func TestTooWideTableIsCut(t *testing.T) {
	md := "| " + strings.Repeat("a | ", 30) + "\n|" + strings.Repeat("---|", 30) + "\n| " + strings.Repeat("word | ", 30) + "\n"
	for _, l := range strings.Split(ansi.Strip(renderMarkdown(md, 60)), "\n") {
		if ansi.StringWidth(l) > 58 {
			t.Errorf("line %d wide: %q", ansi.StringWidth(l), l)
		}
	}
}
