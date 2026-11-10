package suggestions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// writeLog writes a log of outward and inward suggestions with the given outcomes.
func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "suggestions.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func line(outcome string, outward bool, at time.Time) string {
	return fmt.Sprintf(`{"time":%q,"suggested":"s","sent":"s","outcome":%q,"outward":%v}`, at.Format(time.RFC3339), outcome, outward)
}

func TestRead(t *testing.T) {
	old := now.AddDate(0, 0, -40)
	p := writeLog(t,
		line("accepted", false, now),
		line("edited", false, now),
		line("ignored", false, old),
		line("ignored", true, now),
		line("ignored", true, old),
		line("edited", true, now),
		`not json`,
		`{"outcome":"maybe"}`,
		`{"time":"2026-11-09T12:00:00Z","event":"outward suggestions held: accept rate 0.0% over 10 entries"}`,
	)
	s, err := Read(p, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.All != (Counts{1, 2, 3}) || s.Outward != (Counts{0, 1, 2}) || s.Inward != (Counts{1, 1, 1}) {
		t.Errorf("counts: all %+v, outward %+v, inward %+v", s.All, s.Outward, s.Inward)
	}
	if s.Recent != 4 || s.Skipped != 2 {
		t.Errorf("recent %d (want 4), skipped %d (want 2: the note isn't malformed)", s.Recent, s.Skipped)
	}
	if r := s.Inward.Rate(s.Inward.Accepted); fmt.Sprintf("%.1f", r) != "33.3" {
		t.Errorf("inward accept rate %.1f", r)
	}
	if (Counts{}).Rate(0) != 0 {
		t.Error("an empty rate isn't 0")
	}
}

func TestHeld(t *testing.T) {
	for _, c := range []struct {
		name     string
		accepted int
		total    int
		held     bool
	}{
		{"no outward entries", 0, 0, false},
		{"9, all ignored: not enough data", 0, 9, false},
		{"10, all ignored", 0, 10, true},
		{"10, 5 accepted", 5, 10, false},
		{"20, 4 accepted (20%)", 4, 20, true},
	} {
		var lines []string
		for i := 0; i < c.total; i++ {
			outcome := "ignored"
			if i < c.accepted {
				outcome = "accepted"
			}
			lines = append(lines, line(outcome, true, now))
		}
		// Inward suggestions, all ignored, don't move the outward gate.
		for i := 0; i < 15; i++ {
			lines = append(lines, line("ignored", false, now))
		}
		held, why := HeldAt(writeLog(t, lines...))
		if held != c.held {
			t.Errorf("%s: held %v, want %v (%s)", c.name, held, c.held, why)
		}
		if held && !strings.Contains(why, "outward suggestions held: accept rate") {
			t.Errorf("%s: says %q", c.name, why)
		}
	}
}

// A missing or empty log holds nothing.
func TestHeldWithoutALog(t *testing.T) {
	if held, _ := HeldAt(filepath.Join(t.TempDir(), "none.jsonl")); held {
		t.Error("held with no log")
	}
	p := filepath.Join(t.TempDir(), "empty.jsonl")
	os.WriteFile(p, nil, 0o600) //nolint:errcheck
	if held, _ := HeldAt(p); held {
		t.Error("held with an empty log")
	}
}
