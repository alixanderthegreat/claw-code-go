package permissions

import (
	"strings"
	"testing"
)

// "always" on git status allows git status again, not rm or git push (#28).
func TestAlwaysCoversTheCommandNotTheTool(t *testing.T) {
	m := NewManager(ModeDefault, nil)
	m.Remember("bash", "git status", DecisionAllow, ScopeAlways)
	for in, want := range map[string]Decision{
		"git status":             DecisionAllow,
		"git status --short":     DecisionAllow,
		"rm -rf /tmp/x":          DecisionAsk,
		"git push origin main":   DecisionAsk,
		"git status && rm -rf x": DecisionAsk,
		"git status | rm":        DecisionAsk,
		"git status $(rm -rf x)": DecisionAsk,
		"git status > ~/.bashrc": DecisionAsk,
		"git status 2>&1":        DecisionAllow,
		"git status 2>/dev/null": DecisionAllow,
		"GIT_DIR=x git status":   DecisionAsk,
	} {
		if got := m.Check("bash", in); got != want {
			t.Errorf("after always on git status, %q: %v, want %v", in, got, want)
		}
	}
	// Every kind in a pipeline was granted, so the pipeline is.
	m.Remember("bash", "go test ./... | tail -5", DecisionAllow, ScopeAlways)
	if got := m.Check("bash", "go test -run X ./internal/... | tail -20"); got != DecisionAllow {
		t.Errorf("a granted pipeline asks again: %v", got)
	}
	if got := m.Check("bash", "go run . | tail"); got != DecisionAsk {
		t.Errorf("go run allowed by always on go test: %v", got)
	}
	// Other tools: the tool, as before.
	m.Remember("read_file", "/a", DecisionAllow, ScopeAlways)
	if got := m.Check("read_file", "/b"); got != DecisionAllow {
		t.Errorf("read_file: %v", got)
	}
}

// A call whose kind can't be told isn't remembered: always on it allows nothing more.
func TestAlwaysNotRememberedForWrappers(t *testing.T) {
	for _, in := range []string{"sudo ls", "xargs rm", "bash -c 'ls'", "find . -delete", "git -C x status", "echo $(id)"} {
		m := NewManager(ModeDefault, nil)
		m.Remember("bash", in, DecisionAllow, ScopeAlways)
		if got := m.Check("bash", in); got != DecisionAsk {
			t.Errorf("%q remembered: %v", in, got)
		}
		if c := Covers("bash", in); !strings.HasPrefix(c, "always isn't offered: ") {
			t.Errorf("Covers(%q) = %q", in, c)
		}
	}
	if c := Covers("bash", "go test ./... | tail -5"); c != "always: go test, tail" {
		t.Errorf("Covers = %q", c)
	}
}

// An ask rule asks every time, even after an always that covers its command (#3, point 3).
func TestAskRuleBeatsRememberedAllow(t *testing.T) {
	rules := &Ruleset{Rules: []Rule{{Tool: "bash", Pattern: "deploy", Decision: DecisionAsk}}}
	m := NewManager(ModeDefault, rules)
	m.Remember("bash", "./deploy.sh", DecisionAllow, ScopeAlways)
	if got := m.Check("bash", "./deploy.sh"); got != DecisionAsk {
		t.Errorf("deploy after always: %v, want ask", got)
	}
}
