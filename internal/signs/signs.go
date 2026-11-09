// Package signs is a starting language: a small, fixed vocabulary that the simplest model can
// speak and every part of a system can read (#35). Like a child's first words, it lets a model say
// what it understands before it can explain it: one sign, or a state and the thing it's about.
//
// A message is `state object [who] [prob]`:
//
//	stuck #67
//	waiting PR#83 operator
//	done run:abc123 worker 0.9
//
// Each word means one thing, defined here once. Grammar is the same rule as GBNF, so a model
// server can constrain a small model's output to it: with the grammar on, it can't say anything
// else.
package signs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A Word is one entry in the vocabulary: the word, and the one thing it means.
type Word struct {
	Name, Means string
}

// States say what is true of a piece of work now.
var States = []Word{
	{"done", "finished and checked (CI green, or the check named)"},
	{"stuck", "tried, and can't go on without something changing"},
	{"waiting", "nothing to do until someone acts; names who"},
	{"need", "asks for something specific: a decision, access, a file"},
	{"blocked", "a rule or fence stops it: not a failure, and not to route around"},
	{"unsure", "can't tell: the honest answer, never a failure"},
}

// Verdicts judge a claim, always with a probability.
var Verdicts = []Word{
	{"yes", "the claim holds"},
	{"partly", "some of it holds"},
	{"no", "the claim doesn't hold"},
}

// Who can be named in a message.
var Who = []Word{
	{"operator", "the person the system serves"},
	{"gordi", "the local agent"},
	{"claude", "the frontier model"},
	{"worker", "the queue worker"},
	{"ci", "the checks"},
}

// A Sign is an emoji with one meaning, typeable by its shortcode.
type Sign struct {
	Emoji, Code, Means string
}

// Signs are the twelve, for a glance or a reaction. Each means one thing only.
var Signs = []Sign{
	{"🟢", ":done:", "done, checked"},
	{"🧱", ":stuck:", "stuck"},
	{"⏳", ":waiting:", "waiting on someone"},
	{"🙋", ":need:", "needs a decision or access"},
	{"🚧", ":blocked:", "blocked by a rule or fence"},
	{"❔", ":unsure:", "unsure"},
	{"✅", ":yes:", "yes"},
	{"❌", ":no:", "no"},
	{"🤔", ":why:", "not yet: explain"},
	{"🔁", ":retry:", "try again"},
	{"🔥", ":broke:", "something broke: CI red, a crash"},
	{"💤", ":idle:", "idle, nothing in the queue"},
}

// SignCode is the emoji for a sign's shortcode, if it is one.
func SignCode(code string) (string, bool) {
	for _, s := range Signs {
		if s.Code == code {
			return s.Emoji, true
		}
	}
	return "", false
}

// A Message is one parsed sentence of the language.
type Message struct {
	State  string
	Object string
	Who    string  // "" when not named
	Prob   float64 // -1 when not given
}

func (m Message) String() string {
	parts := []string{m.State, m.Object}
	if m.Who != "" {
		parts = append(parts, m.Who)
	}
	if m.Prob >= 0 {
		parts = append(parts, strconv.FormatFloat(m.Prob, 'f', -1, 64))
	}
	return strings.Join(parts, " ")
}

var (
	objectRE = regexp.MustCompile(`^(#[0-9]+|PR#[0-9]+|file:\S+|repo:[a-z0-9-]+|run:[a-z0-9]+)$`)
	probRE   = regexp.MustCompile(`^0\.[0-9][0-9]?$`)
)

func known(ws []Word, s string) bool {
	for _, w := range ws {
		if w.Name == s {
			return true
		}
	}
	return false
}

// Parse reads one message, and says which word is wrong when it isn't one.
func Parse(s string) (Message, error) {
	f := strings.Fields(s)
	if len(f) < 2 || len(f) > 4 {
		return Message{}, fmt.Errorf("want \"state object [who] [prob]\", got %d words", len(f))
	}
	m := Message{State: f[0], Object: f[1], Prob: -1}
	if !known(States, m.State) {
		return Message{}, fmt.Errorf("%q is not a state", m.State)
	}
	if !objectRE.MatchString(m.Object) {
		return Message{}, fmt.Errorf("%q is not an object (#N, PR#N, file:, repo:, run:)", m.Object)
	}
	for _, w := range f[2:] {
		switch {
		case m.Who == "" && m.Prob < 0 && known(Who, w):
			m.Who = w
		case m.Prob < 0 && probRE.MatchString(w):
			m.Prob, _ = strconv.ParseFloat(w, 64)
		default:
			return Message{}, fmt.Errorf("%q is not a who or a probability (0.0-0.99)", w)
		}
	}
	return m, nil
}

func alt(ws []Word) string {
	q := make([]string, len(ws))
	for i, w := range ws {
		q[i] = strconv.Quote(w.Name)
	}
	return strings.Join(q, " | ")
}

// Grammar is Parse's rule as GBNF (llama.cpp's grammar format), built from the same word lists.
var Grammar = `root   ::= state " " object (" " who)? (" " prob)?
state  ::= ` + alt(States) + `
object ::= "#" [0-9]+ | "PR#" [0-9]+ | "file:" [^ \n]+ | "repo:" [a-z0-9-]+ | "run:" [a-z0-9]+
who    ::= ` + alt(Who) + `
prob   ::= "0." [0-9] [0-9]?
`

// VerdictGrammar constrains a judgment to one verdict and its probability: "no 0.85".
var VerdictGrammar = `root ::= (` + alt(Verdicts) + `) " 0." [0-9] [0-9]?
`
