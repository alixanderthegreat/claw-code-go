package permissions

import "sync"

// Manager enforces permissions for tool execution according to a mode and ruleset.
type Manager struct {
	Mode  PermissionMode
	Rules *Ruleset

	mu    sync.Mutex
	cache map[string]Decision // ScopeAlways grants, keyed by tool and kind of command (grantKey)
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
//   - BypassPermissions → Allow, UNLESS a rule denies or asks for this call (ask: #3)
//     (kata 84 Test 2: bypass used to short-circuit before ever consulting the
//     ruleset, so there was no way to auto-allow everything except a specific
//     tool - the exact shape unattended dispatch needs, e.g. deny web_fetch/
//     web_search while still allowing bash/read/write/edit with no prompting)
//   - Plan             → always Deny
//   - Otherwise        → a deny rule, an ask rule, then the session cache, then the ruleset, then Ask
func (m *Manager) CheckRule(tool, input string) (Decision, *Rule) {
	rule := m.Rules.MatchRule(tool, input)
	denied := rule != nil && rule.Decision == DecisionDeny

	switch m.Mode {
	case ModeBypassPermissions:
		// Deny and ask rules hold in bypass (#3): "work freely, but stop before anything
		// leaves the machine" is an ask rule on the outward commands.
		if rule != nil && (rule.Decision == DecisionDeny || rule.Decision == DecisionAsk) {
			return rule.Decision, rule
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

	// An ask rule asks every time: a remembered "always" doesn't carry over to it (#3, point 3).
	if rule != nil && rule.Decision == DecisionAsk {
		return DecisionAsk, rule
	}

	if d, ok := m.remembered(tool, input); ok {
		return d, nil
	}

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

// Remember stores a user decision in the session cache for ScopeAlways grants: for the kinds of
// command in input (Kinds), not the whole tool (#28). ScopeOnce decisions, and a call whose kinds
// can't be told, are not cached.
func (m *Manager) Remember(tool, input string, decision Decision, scope Scope) {
	if scope != ScopeAlways {
		return
	}
	kinds, _ := Kinds(tool, input)
	m.mu.Lock()
	for _, k := range kinds {
		m.cache[grantKey(tool, k)] = decision
	}
	m.mu.Unlock()
}

// remembered is the cached decision for a call: found only when every kind in it was remembered,
// and a deny for any of them wins.
func (m *Manager) remembered(tool, input string) (Decision, bool) {
	kinds, _ := Kinds(tool, input)
	if kinds == nil {
		return 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d := DecisionAllow
	for _, k := range kinds {
		c, ok := m.cache[grantKey(tool, k)]
		if !ok {
			return 0, false
		}
		if c == DecisionDeny {
			d = DecisionDeny
		}
	}
	return d, true
}

func grantKey(tool, kind string) string { return tool + "\x00" + kind }
