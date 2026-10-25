package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

const maxHistoryEntries = 500

// inputHistory maintains a ring buffer of submitted messages for ↑/↓ navigation.
type inputHistory struct {
	entries []string
	pos     int    // navigation index; -1 when not navigating
	draft   string // text saved when navigation begins
	path    string // where entries are kept across restarts; "" keeps them in memory only
}

func newInputHistory() *inputHistory {
	return &inputHistory{pos: -1}
}

// historyPath is the input history's file, beside the sessions directory:
// ~/.claw-code/input_history.jsonl. No sessions directory, no file.
func historyPath(sessionDir string) string {
	if sessionDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(sessionDir), "input_history.jsonl")
}

// loadInputHistory is the history kept at path - one JSON string per line, so
// an entry can hold newlines - and keeps adding to it. ↑ used to find nothing
// after a restart. A file that has grown past twice the cap is rewritten with
// only the newest entries.
func loadInputHistory(path string) *inputHistory {
	h := &inputHistory{pos: -1, path: path}
	f, err := os.Open(path)
	if err != nil {
		return h
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	lines := 0
	for sc.Scan() {
		var e string
		if json.Unmarshal(sc.Bytes(), &e) == nil && e != "" {
			h.entries = append(h.entries, e)
			lines++
		}
	}
	f.Close()
	if len(h.entries) > maxHistoryEntries {
		h.entries = h.entries[len(h.entries)-maxHistoryEntries:]
	}
	if lines > 2*maxHistoryEntries {
		h.rewrite()
	}
	return h
}

// rewrite replaces the file with the entries held now.
func (h *inputHistory) rewrite() {
	tmp := h.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range h.entries {
		b, _ := json.Marshal(e)
		w.Write(append(b, '\n'))
	}
	if w.Flush() == nil && f.Close() == nil {
		os.Rename(tmp, h.path)
	}
}

// save appends one entry to the file.
func (h *inputHistory) save(s string) {
	if h.path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(h.path), 0o700)
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(s)
	f.Write(append(b, '\n'))
}

// Push adds a new entry. Ignores empty strings and consecutive duplicates.
func (h *inputHistory) Push(s string) {
	if s == "" {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == s {
		return
	}
	h.entries = append(h.entries, s)
	if len(h.entries) > maxHistoryEntries {
		h.entries = h.entries[1:]
	}
	h.save(s)
	h.pos = -1
}

// Prev navigates to the previous (older) entry.
// currentText is saved as a draft when navigation begins.
func (h *inputHistory) Prev(currentText string) string {
	if len(h.entries) == 0 {
		return currentText
	}
	if h.pos == -1 {
		h.draft = currentText
		h.pos = len(h.entries) - 1
	} else if h.pos > 0 {
		h.pos--
	}
	return h.entries[h.pos]
}

// Next navigates to the next (newer) entry; returns the draft when past the newest.
func (h *inputHistory) Next() string {
	if h.pos == -1 {
		return ""
	}
	h.pos++
	if h.pos >= len(h.entries) {
		h.pos = -1
		return h.draft
	}
	return h.entries[h.pos]
}

// Navigating reports whether ↑/↓ are stepping through entries.
func (h *inputHistory) Navigating() bool {
	return h.pos != -1
}

// Reset exits navigation mode. Call after any non-navigation edit.
func (h *inputHistory) Reset() {
	h.pos = -1
	h.draft = ""
}
