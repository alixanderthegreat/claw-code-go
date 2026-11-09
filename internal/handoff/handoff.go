// Package handoff finds the condensed handoffs a nightly condenser (simple-condense) writes for
// Claude Code's sessions, so Gordi can pick up where a long session left off: /handoff loads the
// latest for the working folder, or one session's by its id prefix.
//
//	<dir>/<project>/latest-handoff.md      the newest session's handoff
//	<dir>/<project>/<session-id>/handoff.md each session's own
//
// <dir> is $CLAW_HANDOFF_DIR, else $XDG_DATA_HOME/simple-condense, else
// ~/.local/share/simple-condense. <project> is the folder as Claude Code names it: every
// character outside A-Z, a-z, 0-9 and '-' becomes '-'.
package handoff

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Dir is where the handoffs live.
func Dir() string {
	if d := os.Getenv("CLAW_HANDOFF_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "simple-condense")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "simple-condense")
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9-]`)

// ProjectKey names a folder the way Claude Code names its project folders.
func ProjectKey(wd string) string { return unsafe.ReplaceAllString(wd, "-") }

// Entry is one session's handoff.
type Entry struct {
	ID      string // the session id
	Path    string
	Written time.Time
	Size    string // the session's size, as the handoff states it ("209.6 MB")
	Turns   string // its operator turns ("1135")
}

var sizeRE = regexp.MustCompile(`^\*\*Original size:\*\* ([^,]+), \d+ messages \((\d+) operator turns`)

// List returns the folder's handoffs, newest first.
func List(dir, wd string) ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(dir, ProjectKey(wd), "*", "handoff.md"))
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		e := Entry{ID: filepath.Base(filepath.Dir(f)), Path: f, Written: st.ModTime()}
		e.Size, e.Turns = header(f)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Written.After(out[j].Written) })
	return out, nil
}

// header reads the size and turns from a handoff's first lines.
func header(path string) (size, turns string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; i < 10 && sc.Scan(); i++ {
		if m := sizeRE.FindStringSubmatch(sc.Text()); m != nil {
			return m[1], m[2]
		}
	}
	return "", ""
}

// ErrNone means the folder has no handoff (or none matching the prefix asked for).
var ErrNone = errors.New("no handoff")

// Load returns the handoff for wd: the session whose id starts with prefix, or with no prefix the
// latest. It returns the entry loaded and every handoff the folder has, newest first.
func Load(dir, wd, prefix string) (string, Entry, []Entry, error) {
	all, err := List(dir, wd)
	if err != nil {
		return "", Entry{}, nil, err
	}
	var pick Entry
	switch {
	case prefix != "":
		for _, e := range all {
			if strings.HasPrefix(e.ID, prefix) {
				pick = e
				break
			}
		}
		if pick.Path == "" {
			return "", Entry{}, all, fmt.Errorf("%w for a session starting %q", ErrNone, prefix)
		}
	default:
		latest := filepath.Join(dir, ProjectKey(wd), "latest-handoff.md")
		st, err := os.Stat(latest)
		if err != nil {
			if len(all) == 0 {
				return "", Entry{}, all, ErrNone
			}
			pick = all[0]
			break
		}
		pick = Entry{ID: "latest", Path: latest, Written: st.ModTime()}
		pick.Size, pick.Turns = header(latest)
		for _, e := range all { // name the session latest-handoff.md was copied from
			if e.Size == pick.Size && e.Turns == pick.Turns && !e.Written.After(pick.Written.Add(time.Minute)) {
				pick.ID = e.ID
				break
			}
		}
	}
	b, err := os.ReadFile(pick.Path)
	if err != nil {
		return "", Entry{}, all, err
	}
	return string(b), pick, all, nil
}

// Find looks for a session's handoff in every folder, by its id prefix: a session id is unique, so
// /handoff ID works from anywhere. It returns the matches, newest first.
func Find(dir, prefix string) ([]Entry, error) {
	if prefix == "" {
		return nil, nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*", prefix+"*", "handoff.md"))
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		e := Entry{ID: filepath.Base(filepath.Dir(f)), Path: f, Written: st.ModTime()}
		e.Size, e.Turns = header(f)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Written.After(out[j].Written) })
	return out, nil
}

// Projects lists the folders that have a handoff, as Claude Code names them, newest first.
func Projects(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*", "latest-handoff.md"))
	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	var out []string
	for _, f := range files {
		out = append(out, filepath.Base(filepath.Dir(f)))
	}
	return out
}

// Folder is the project name a handoff's path sits under.
func Folder(e Entry) string { return filepath.Base(filepath.Dir(filepath.Dir(e.Path))) }

// Short is an id cut to the length /handoff takes.
func Short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Describe is one line about an entry: "b4a3e097  2026-11-08 18:57  209.6 MB, 1135 turns".
func Describe(e Entry) string {
	s := fmt.Sprintf("%-8s  %s", Short(e.ID), e.Written.Local().Format("2006-01-02 15:04"))
	if e.Size != "" {
		s += fmt.Sprintf("  %s, %s turns", e.Size, e.Turns)
	}
	return s
}

// Local says whether a model endpoint is on this machine or the home network. A handoff can hold
// summaries of client material, so it goes only to a local model; an empty base URL is a hosted
// provider (Anthropic's or OpenAI's, by /login).
func Local(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if baseURL == "" || err != nil || u.Hostname() == "" {
		return false
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() && !ip.IsPrivate() {
			return false
		}
	}
	return true
}
