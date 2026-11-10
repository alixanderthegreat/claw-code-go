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
	"math"
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
	Probes               int // outward suggestions shown while held, to keep learning (#57)
}

// entry is one line of the log, as tui's logSuggestion writes it.
type entry struct {
	Time    string `json:"time"`
	Outcome string `json:"outcome"`
	Outward bool   `json:"outward"`
	Probe   bool   `json:"probe"` // shown while outward ones were held (#57)
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
		if e.Probe {
			s.Probes++
		}
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

// The gate (#57): outward suggestions are held once there are MinOutward of them logged and they
// do clearly worse than inward ones - the top of their accept rate's 95% Wilson interval below the
// inward rate. A fixed bar (50%, #50) held them whatever they did, since inward suggestions don't
// reach it either; comparing the two asks the trial's question: is an outward suggestion taken
// less than any other?
const MinOutward = 10

// ProbeEvery is how often an outward suggestion is still shown while they're held: one in this
// many. Held with none shown, nothing new is logged and the gate could never change its mind.
const ProbeEvery = 5

// wilsonHigh is the top of the 95% Wilson score interval for k of n, as a percentage.
func wilsonHigh(k, n int) float64 {
	if n == 0 {
		return 100
	}
	const z = 1.96
	p, nf := float64(k)/float64(n), float64(n)
	den := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / den
	margin := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / den
	return 100 * math.Min(1, center+margin)
}

// Held says whether outward suggestions should be held back, and why, given the outward and
// inward counts. Under MinOutward outward entries there isn't enough evidence, so they're offered.
func Held(outward, inward Counts) (bool, string) {
	n := outward.Total()
	if n < MinOutward {
		return false, fmt.Sprintf("outward suggestions offered: %d of %d entries needed to decide", n, MinOutward)
	}
	rate, high, in := outward.Rate(outward.Accepted), wilsonHigh(outward.Accepted, n), inward.Rate(inward.Accepted)
	if high < in {
		return true, fmt.Sprintf("outward suggestions held (1 in %d still shown): accepted %.1f%% of %d, at most %.1f%%, below inward's %.1f%%",
			ProbeEvery, rate, n, high, in)
	}
	return false, fmt.Sprintf("outward suggestions offered: accepted %.1f%% of %d, not clearly below inward's %.1f%% (could be up to %.1f%%)",
		rate, n, in, high)
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
	return Held(s.Outward, s.Inward)
}
