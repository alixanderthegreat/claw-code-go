package signs

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string // the message back, or "" when it must fail
	}{
		{"stuck #67", "stuck #67"},
		{"waiting PR#83 operator", "waiting PR#83 operator"},
		{"done run:abc123 worker 0.9", "done run:abc123 worker 0.9"},
		{"unsure file:worker.go 0.6", "unsure file:worker.go 0.6"},
		{"blocked repo:simple-worker", "blocked repo:simple-worker"},
		{"stuck", ""},                    // no object
		{"finished #67", ""},             // not a state
		{"stuck 67", ""},                 // not an object
		{"stuck #67 mary", ""},           // not a who
		{"stuck #67 0.95 operator", ""},  // who after prob
		{"stuck #67 1.0", ""},            // prob out of range
		{"stuck #67 operator gordi", ""}, // two whos
		{"stuck #67 is broken now ok", ""},
	} {
		m, err := Parse(c.in)
		if c.want == "" {
			if err == nil {
				t.Errorf("Parse(%q) = %v, want an error", c.in, m)
			}
			continue
		}
		if err != nil || m.String() != c.want {
			t.Errorf("Parse(%q) = %v, %v; want %q", c.in, m, err, c.want)
		}
	}
}

func TestEveryWordMeansSomething(t *testing.T) {
	seen := map[string]bool{}
	for _, ws := range [][]Word{States, Verdicts, Who} {
		for _, w := range ws {
			if w.Means == "" || seen[w.Name] {
				t.Errorf("word %q: empty meaning or defined twice", w.Name)
			}
			seen[w.Name] = true
		}
	}
	emoji, codes := map[string]bool{}, map[string]bool{}
	for _, s := range Signs {
		if s.Means == "" || emoji[s.Emoji] || codes[s.Code] {
			t.Errorf("sign %s %s: empty meaning, or emoji or code used twice", s.Emoji, s.Code)
		}
		emoji[s.Emoji], codes[s.Code] = true, true
	}
	if len(Signs) != 11 {
		t.Errorf("%d signs, want the eleven", len(Signs))
	}
}

// The grammar is built from the same lists Parse checks, so a word added to one is in both.
func TestGrammarHasEveryWord(t *testing.T) {
	for _, ws := range [][]Word{States, Who} {
		for _, w := range ws {
			if !strings.Contains(Grammar, `"`+w.Name+`"`) {
				t.Errorf("grammar lacks %q", w.Name)
			}
		}
	}
	for _, w := range Verdicts {
		if !strings.Contains(VerdictGrammar, `"`+w.Name+`"`) {
			t.Errorf("verdict grammar lacks %q", w.Name)
		}
	}
}

func TestSignCode(t *testing.T) {
	if e, ok := SignCode(":stuck:"); !ok || e != "🧱" {
		t.Errorf(":stuck: = %q, %v", e, ok)
	}
	if _, ok := SignCode(":fire:"); ok {
		t.Error(":fire: isn't a sign")
	}
}

// The decision signs are single code points (the format rule), distinct from the eleven, and
// their shortcodes resolve.
func TestDecisions(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Signs {
		seen[s.Emoji], seen[s.Code] = true, true
	}
	for _, d := range Decisions {
		if n := len([]rune(d.Emoji)); n != 1 {
			t.Errorf("%s is %d code points", d.Emoji, n)
		}
		if seen[d.Emoji] || seen[d.Code] {
			t.Errorf("%s %s repeats a sign", d.Emoji, d.Code)
		}
		seen[d.Emoji], seen[d.Code] = true, true
		if e, ok := SignCode(d.Code); !ok || e != d.Emoji {
			t.Errorf("%s doesn't resolve", d.Code)
		}
	}
}
