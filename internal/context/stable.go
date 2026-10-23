package context

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stable.go: the context that doesn't change from one message to the next - the instruction files
// (CLAUDE.md, AGENTS.md), the skills on offer, and the memory index - for the system prompt, where
// it's sent once and cached. It used to ride at the head of every user message with the clock and
// git status, so each message carried another copy of the whole CLAUDE.md into the history.

const (
	maxInstructionBytes = 20 * 1024
	maxMemoryIndexBytes = 8 * 1024
)

// instructionNames are the files read from each directory, in this order.
var instructionNames = []string{"CLAUDE.md", "AGENTS.md", filepath.Join(".claude", "CLAUDE.md")}

// dirsDown is the directories from home (or the filesystem root, outside home) down to workDir,
// outermost first - so a project's own instructions come after, and refine, those above it.
func dirsDown(workDir string) []string {
	home, _ := os.UserHomeDir()
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return []string{workDir}
	}
	var up []string
	for d := abs; ; d = filepath.Dir(d) {
		up = append(up, d)
		if d == home || d == filepath.Dir(d) {
			break
		}
	}
	for i, j := 0, len(up)-1; i < j; i, j = i+1, j-1 {
		up[i], up[j] = up[j], up[i]
	}
	return up
}

// instructionPaths is every instruction file that applies to workDir: the user's global one, then
// each directory's from home down.
func instructionPaths(workDir string) []string {
	home, _ := os.UserHomeDir()
	paths := []string{filepath.Join(home, ".claude", "CLAUDE.md")}
	for _, d := range dirsDown(workDir) {
		for _, n := range instructionNames {
			p := filepath.Join(d, n)
			if p != paths[0] {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// Instructions is the text of the instruction files that exist, each headed by its path.
func Instructions(workDir string) string {
	var parts []string
	total := 0
	for _, p := range instructionPaths(workDir) {
		data, err := os.ReadFile(p)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		text := string(data)
		if room := maxInstructionBytes - total; len(text) > room {
			if room <= 0 {
				break
			}
			text = text[:room] + "\n... (truncated)"
		}
		total += len(text)
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", tildePath(p), strings.TrimSpace(text)))
	}
	return strings.Join(parts, "\n\n")
}

// Skill is one SKILL.md: what it's for, and where to read it.
type Skill struct {
	Name, Description, Path string
}

// skillDirs is where skills live: the user's, then each directory's .claude/skills from home down.
func skillDirs(workDir string) []string {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".claude", "skills")}
	for _, d := range dirsDown(workDir) {
		if s := filepath.Join(d, ".claude", "skills"); s != dirs[0] {
			dirs = append(dirs, s)
		}
	}
	return dirs
}

// Skills finds the skills that apply to workDir, a nearer one replacing a same-named one further up.
func Skills(workDir string) []Skill {
	byName := map[string]Skill{}
	for _, dir := range skillDirs(workDir) {
		matches, _ := filepath.Glob(filepath.Join(dir, "*", "SKILL.md"))
		for _, p := range matches {
			name, desc := skillHeader(p)
			if name == "" {
				name = filepath.Base(filepath.Dir(p))
			}
			if desc == "" {
				continue // without a description there's no telling when to use it
			}
			byName[name] = Skill{Name: name, Description: desc, Path: p}
		}
	}
	out := make([]Skill, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// skillHeader reads name and description from a SKILL.md's frontmatter.
func skillHeader(path string) (name, desc string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return "", ""
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "name":
			name = v
		case "description":
			desc = v
		}
	}
	if sc.Err() != nil {
		return "", ""
	}
	return name, desc
}

// MemoryDir is where memories are kept: one file each, indexed by MEMORY.md.
func MemoryDir() string {
	if d := os.Getenv("CLAW_MEMORY_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claw-code", "memory")
}

// Stable is the system prompt's context block: instructions, skills, memory.
func Stable(workDir string) string {
	var sections []string
	if ins := Instructions(workDir); ins != "" {
		sections = append(sections, "# Instructions\n\nFollow these; the ones further down are closer to this project and win.\n\n"+ins)
	}
	if skills := Skills(workDir); len(skills) > 0 {
		var b strings.Builder
		b.WriteString("# Skills\n\nWhen a task matches one, read its file with read_file first and follow it.\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "\n- %s: %s (%s)", s.Name, s.Description, tildePath(s.Path))
		}
		sections = append(sections, b.String())
	}
	dir := MemoryDir()
	index := ""
	if data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md")); err == nil {
		index = strings.TrimSpace(string(data))
		if len(index) > maxMemoryIndexBytes {
			index = index[:maxMemoryIndexBytes] + "\n... (truncated)"
		}
	}
	mem := "# Memory\n\n" +
		"Facts worth keeping across sessions - who the user is, how they like to work, decisions, where things are - live in " +
		tildePath(dir) + ", one file each, listed one line apiece in its MEMORY.md (`- [title](file.md) - one-line summary`). " +
		"Save one with write_file when you learn something that will matter again and isn't in the code or git history, and add its line; " +
		"update or remove a memory that turns out wrong. Read a memory's file when its line is relevant."
	if index != "" {
		mem += "\n\nMEMORY.md:\n" + index
	} else {
		mem += "\n\n(No memories yet.)"
	}
	sections = append(sections, mem)
	return strings.Join(sections, "\n\n")
}

// StableMtimes fingerprints everything Stable reads, so a cache knows when to rebuild it.
func StableMtimes(workDir string) map[string]int64 {
	out := map[string]int64{}
	stat := func(p string) {
		if info, err := os.Stat(p); err == nil {
			out[p] = info.ModTime().UnixNano()
		}
	}
	for _, p := range instructionPaths(workDir) {
		stat(p)
	}
	for _, d := range skillDirs(workDir) {
		stat(d)
		matches, _ := filepath.Glob(filepath.Join(d, "*", "SKILL.md"))
		for _, p := range matches {
			stat(p)
		}
	}
	stat(filepath.Join(MemoryDir(), "MEMORY.md"))
	return out
}

// tildePath shows a path under home as ~/...
func tildePath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
