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
		name         string
		outAcc, outN int
		inAcc, inN   int
		held         bool
	}{
		{"no outward entries", 0, 0, 25, 79, false},
		{"9, all ignored: not enough data", 0, 9, 25, 79, false},
		{"10 ignored, inward never taken either", 0, 10, 0, 15, false},
		{"10 ignored, inward taken a third of the time", 0, 10, 30, 90, true},
		{"today's log: 3 of 17 against 25 of 79", 3, 17, 25, 79, false},
		{"2 of 30 against 30 of 100", 2, 30, 30, 100, true},
		{"as good as inward", 10, 30, 30, 100, false},
	} {
		var lines []string
		add := func(acc, n int, outward bool) {
			for i := 0; i < n; i++ {
				outcome := "ignored"
				if i < acc {
					outcome = "accepted"
				}
				lines = append(lines, line(outcome, outward, now))
			}
		}
		add(c.outAcc, c.outN, true)
		add(c.inAcc, c.inN, false)
		held, why := HeldAt(writeLog(t, lines...))
		if held != c.held {
			t.Errorf("%s: held %v, want %v (%s)", c.name, held, c.held, why)
		}
		if held && !strings.Contains(why, "below inward's") {
			t.Errorf("%s: says %q", c.name, why)
		}
	}
}

func TestProbesCounted(t *testing.T) {
	p := writeLog(t, line("ignored", true, now), `{"time":"`+now.Format(time.RFC3339)+`","outcome":"accepted","outward":true,"probe":true}`)
	s, err := Read(p, now)
	if err != nil || s.Probes != 1 || s.Outward.Total() != 2 {
		t.Errorf("stats = %+v, %v; want 1 probe of 2 outward", s, err)
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
