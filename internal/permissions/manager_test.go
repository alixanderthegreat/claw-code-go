package permissions

import "testing"

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
