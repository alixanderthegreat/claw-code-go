package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAcceptEditsAllowsEdits: accept-edits lets every reading and editing tool
// through without asking - file_edit included, which it used to leave out - and
// still asks before bash.
func TestAcceptEditsAllowsEdits(t *testing.T) {
	m := NewManager(ModeAcceptEdits, nil)
	for _, tool := range []string{"read_file", "glob", "grep", "write_file", "file_edit"} {
		if d := m.Check(tool, "internal/x.go"); d != DecisionAllow {
			t.Errorf("%s: %v, want allow", tool, d)
		}
	}
	if d := m.Check("bash", "rm -rf build"); d != DecisionAsk {
		t.Errorf("bash: %v, want ask", d)
	}
}

// TestDefaultModeAsks: in the default mode an edit asks, as before.
func TestDefaultModeAsks(t *testing.T) {
	if d := NewManager(ModeDefault, nil).Check("file_edit", "internal/x.go"); d != DecisionAsk {
		t.Errorf("file_edit in default mode: %v, want ask", d)
	}
}

// TestDenyRuleBeatsRememberedAllow: allowing bash for the session ("always") doesn't switch off
// a rule that denies one kind of bash command; other bash still runs without asking.
func TestDenyRuleBeatsRememberedAllow(t *testing.T) {
	rules := &Ruleset{Rules: []Rule{{Tool: "bash", Pattern: "git push", Decision: DecisionDeny}}}
	m := NewManager(ModeDefault, rules)
	m.Remember("bash", "", DecisionAllow, ScopeAlways)
	if d := m.Check("bash", "git commit -m x && git push origin main"); d != DecisionDeny {
		t.Errorf("git push after an always-allow: %v, want deny", d)
	}
	if d := m.Check("bash", "go test ./..."); d != DecisionAllow {
		t.Errorf("go test after an always-allow: %v, want allow", d)
	}
}

// TestLoadLayeredUserFirst: the user's rules come before the project's, so a project allow can't
// override a user deny; a missing layer is empty, not an error.
func TestLoadLayeredUserFirst(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.json")
	project := filepath.Join(dir, "project.json")
	os.WriteFile(user, []byte(`{"rules":[{"tool":"bash","pattern":"deploy","decision":"deny"}]}`), 0o600)
	os.WriteFile(project, []byte(`{"rules":[{"tool":"bash","pattern":"deploy","decision":"allow"}]}`), 0o600)

	rs, err := LoadLayered(user, project)
	if err != nil {
		t.Fatal(err)
	}
	if d := NewManager(ModeBypassPermissions, rs).Check("bash", "./deploy.sh"); d != DecisionDeny {
		t.Errorf("deploy with a user deny and a project allow: %v, want deny", d)
	}

	rs, err = LoadLayered(filepath.Join(dir, "missing.json"), project)
	if err != nil || len(rs.Rules) != 1 {
		t.Errorf("missing user layer: %d rules, err %v; want 1 rule, no error", len(rs.Rules), err)
	}
}
