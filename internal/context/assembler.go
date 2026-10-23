package context

import (
	"strings"
	"sync"
)

// Assembler collects project context: the stable part for the system prompt (Stable - instructions,
// skills, memory), and what changes each turn for the head of the user's message (Assemble).
type Assembler struct {
	WorkDir string

	mu           sync.Mutex
	stableCache  string
	stableMtimes map[string]int64
}

// NewAssembler creates an Assembler for the given working directory.
func NewAssembler(workDir string) *Assembler {
	return &Assembler{WorkDir: workDir}
}

// Assemble returns what changes from turn to turn - environment info (the clock) and git status -
// for the head of the user's message. Any individual failure is silently skipped.
func (a *Assembler) Assemble() string {
	var sections []string

	if info := SystemInfo(a.WorkDir); info != "" {
		sections = append(sections, "# Environment\n\n"+info)
	}

	if git := GitStatus(a.WorkDir); git != "" {
		sections = append(sections, "# Git Status\n\n"+git)
	}

	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n\n")
}

// Stable returns the system prompt's context block (stable.go), re-read only when a file it comes
// from changes - so the system prompt, and the cache behind it, hold still between edits.
func (a *Assembler) Stable() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	current := StableMtimes(a.WorkDir)
	if a.stableMtimes == nil || !mtimesEqual(current, a.stableMtimes) {
		a.stableCache = Stable(a.WorkDir)
		a.stableMtimes = current
	}
	return a.stableCache
}

func mtimesEqual(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
