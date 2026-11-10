// Package suggestions reads the log of next-prompt suggestions (#32, #50): how often each was
// accepted, edited or ignored, split by whether it looked outward, and the gate those rates settle -
// whether an outward suggestion is still offered.
package suggestions

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Path is where each suggestion and its outcome go, on this machine only.
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local/share/claw-code-go/suggestions.jsonl")
}

// Counts are the outcomes of some suggestions.
type Counts struct {
	Accepted, Edited, Ignored int
}

// Total is how many suggestions were counted.
func (c Counts) Total() int { return c.Accepted + c.Edited + c.Ignored }

// Rate is n as a percentage of the total, 0 when there's none.
func (c Counts) Rate(n int) float64 {
	if c.Total() == 0 {
		return 0
	}
	return 100 * float64(n) / float64(c.Total())
}

func (c *Counts) add(outcome string) {
	switch outcome {
	case "accepted":
		c.Accepted++
	case "edited":
		c.Edited++
	case "ignored":
		c.Ignored++
	}
}

// Stats are the whole log's outcomes, split by whether a suggestion looked outward.
type Stats struct {
	All, Outward, Inward Counts
	Recent               int // of All, logged in the 30 days before now
	Skipped              int // lines that aren't a suggestion's outcome
}

// entry is one line of the log, as tui's logSuggestion writes it.
type entry struct {
	Time    string `json:"time"`
	Outcome string `json:"outcome"`
	Outward bool   `json:"outward"`
	Event   string `json:"event"` // a note about the gate, not a suggestion (see tui's outwardHeld)
}

// Read counts the log at path. A missing file is an error that errors.Is(err, os.ErrNotExist).
func Read(path string, now time.Time) (Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return Stats{}, err
	}
	defer f.Close()
	return Count(f, now)
}

// Count reads a log from r. A line that isn't JSON, or has no known outcome, is skipped and counted.
func Count(r io.Reader, now time.Time) (Stats, error) {
	var s Stats
	since := now.AddDate(0, 0, -30)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e entry
		err := json.Unmarshal(line, &e)
		if err == nil && e.Event != "" {
			continue
		}
		if err != nil || (e.Outcome != "accepted" && e.Outcome != "edited" && e.Outcome != "ignored") {
			s.Skipped++
			continue
		}
		s.All.add(e.Outcome)
		if e.Outward {
			s.Outward.add(e.Outcome)
		} else {
			s.Inward.add(e.Outcome)
		}
		if t, err := time.Parse(time.RFC3339, e.Time); err == nil && !t.Before(since) {
			s.Recent++
		}
	}
	return s, sc.Err()
}

// The gate: outward suggestions are held once there are MinOutward of them logged and fewer than
// HoldBelow percent were accepted as suggested.
const (
	MinOutward = 10
	HoldBelow  = 50.0
)

// Held says whether outward suggestions should be held back, and why, given the outward counts.
// Under MinOutward entries there isn't enough evidence, so they're offered.
func Held(outward Counts) (bool, string) {
	n := outward.Total()
	if n < MinOutward {
		return false, fmt.Sprintf("outward suggestions offered: %d of %d entries needed to decide", n, MinOutward)
	}
	rate := outward.Rate(outward.Accepted)
	if rate < HoldBelow {
		return true, fmt.Sprintf("outward suggestions held: accept rate %.1f%% over %d entries", rate, n)
	}
	return false, fmt.Sprintf("outward suggestions offered: accept rate %.1f%% over %d entries", rate, n)
}

// HeldAt reads the log at path and applies the gate. A missing, empty or unreadable log holds
// nothing.
func HeldAt(path string) (bool, string) {
	s, err := Read(path, time.Now())
	if errors.Is(err, os.ErrNotExist) {
		return false, "outward suggestions offered: no suggestions logged yet"
	}
	if err != nil {
		return false, "outward suggestions offered: " + err.Error()
	}
	return Held(s.Outward)
}
