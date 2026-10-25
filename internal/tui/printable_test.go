package tui

import "testing"

func TestPrintable(t *testing.T) {
	for in, want := range map[string]string{
		"plain text\n\tindented":   "plain text\n\tindented",
		"red \x1b[31mword\x1b[0m":  "red [31mword[0m",
		"over\rwrite\a":            "overwrite",
		"bad \xff\xfe bytes":       "bad � bytes",
		"keeps unicode: héllo → ✓": "keeps unicode: héllo → ✓",
		"c1 \u0085 control":        "c1  control",
	} {
		if got := printable(in); got != want {
			t.Errorf("printable(%q) = %q, want %q", in, got, want)
		}
	}
}
