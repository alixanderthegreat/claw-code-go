package permissions

import "sync"

// Manager enforces permissions for tool execution according to a mode and ruleset.
type Manager struct {
	Mode  PermissionMode
	Rules *Ruleset

	mu    sync.Mutex
	cache map[string]Decision // keyed by tool name for ScopeAlways grants
}

// NewManager creates a Manager with the given mode and ruleset.
// If rules is nil an empty ruleset is used.
func NewManager(mode PermissionMode, rules *Ruleset) *Manager {
	if rules == nil {
		rules = &Ruleset{}
	}
	return &Manager{
		Mode:  mode,
		Rules: rules,
		cache: make(map[string]Decision),
	}
}

// Check returns the Decision for the given tool and input summary; CheckRule says which rule
// decided it.
func (m *Manager) Check(tool, input string) Decision {
	d, _ := m.CheckRule(tool, input)
	return d
}

// CheckRule returns the Decision for the given tool and input summary, and the rule that decided
// it (nil when no rule did: a mode, a remembered answer, or nothing matched).
//
//   - BypassPermissions → Allow, UNLESS the ruleset explicitly denies this tool
//     (kata 84 Test 2: bypass used to short-circuit before ever consulting the
//     ruleset, so there was no way to auto-allow everything except a specific
//     tool - the exact shape unattended dispatch needs, e.g. deny web_fetch/
//     web_search while still allowing bash/read/write/edit with no prompting)
//   - Plan             → always Deny
//   - Otherwise        → a deny rule, then the session cache, then the ruleset, then Ask
func (m *Manager) CheckRule(tool, input string) (Decision, *Rule) {
	rule := m.Rules.MatchRule(tool, input)
	denied := rule != nil && rule.Decision == DecisionDeny

	switch m.Mode {
	case ModeBypassPermissions:
		if denied {
			return DecisionDeny, rule
		}
		return DecisionAllow, nil
	case ModePlan:
		return DecisionDeny, nil
	}

	// A deny rule wins over a remembered "always": allowing bash for the session mustn't
	// switch off a rule that denies one kind of bash command.
	if denied {
		return DecisionDeny, rule
	}

	m.mu.Lock()
	if d, ok := m.cache[tool]; ok {
		m.mu.Unlock()
		return d, nil
	}
	m.mu.Unlock()

	if rule != nil {
		return rule.Decision, rule
	}

	// AcceptEdits: auto-allow read-only and edit tools, ask for bash. file_edit
	// was left out, so every edit still asked in the mode named for them.
	if m.Mode == ModeAcceptEdits {
		switch tool {
		case "read_file", "glob", "grep", "write_file", "file_edit":
			return DecisionAllow, nil
		}
	}

	return DecisionAsk, nil
}

// Remember stores a user decision in the session cache for ScopeAlways grants.
// ScopeOnce decisions are not cached.
func (m *Manager) Remember(tool, _ string, decision Decision, scope Scope) {
	if scope != ScopeAlways {
		return
	}
	m.mu.Lock()
	m.cache[tool] = decision
	m.mu.Unlock()
}
