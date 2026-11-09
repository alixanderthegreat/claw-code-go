package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestShortcodeQuery(t *testing.T) {
	for before, want := range map[string]string{
		"ship :chec":    "chec",
		":fi":           "fi",
		"(:st":          "st",
		"line\n:sm":     "sm",
		"ship :c":       "", // one letter
		"meet at 12:30": "", // not at a word's start
		"a:bc":          "", // not at a word's start
		"run `x :chec":  "", // inside backticks
		"ship :chec:":   "", // closed
		"ship :Chec":    "", // names are lower case
		"ship :chec ok": "", // a space ended it
	} {
		if got := shortcodeQuery([]rune(before)); got != want {
			t.Errorf("shortcodeQuery(%q) = %q, want %q", before, got, want)
		}
	}
}

func TestEmojiMatches(t *testing.T) {
	ms := emojiMatches("chec", completeMax)
	var codes []string
	for _, m := range ms {
		codes = append(codes, m.Code)
	}
	if !strings.Contains(strings.Join(codes, " "), ":checkered_flag:") {
		t.Errorf(":chec -> %v, want :checkered_flag: among them", codes)
	}
	if ms := emojiMatches("checkered", completeMax); len(ms) == 0 || ms[0].Emoji != "🏁" {
		t.Errorf(":checkered -> %v, want 🏁 first", ms)
	}
	// "chec" reaches ☑ through two codes, one with the colour selector: offered once.
	n := 0
	for _, m := range ms {
		if strings.TrimSuffix(m.Emoji, "\ufe0f") == "☑" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("☑ offered %d times in %v", n, codes)
	}
	if st := emojiMatches("st", completeMax); len(st) == 0 || st[0].Code != ":stuck:" {
		t.Errorf(":st -> %v, want the sign :stuck: first", st)
	}
	seen := map[string]bool{}
	for _, m := range emojiMatches("smile", completeMax) {
		if seen[m.Emoji] {
			t.Errorf("%s offered twice", m.Emoji)
		}
		seen[m.Emoji] = true
	}
	if len(emojiMatches("e", 100)) > 100 || len(emojiMatches("a", completeMax)) != completeMax {
		t.Error("the row isn't capped")
	}
	if len(emojiMatches("zzqx", completeMax)) != 0 {
		t.Error("nonsense matches something")
	}
}

func TestEmojiCompleteTyped(t *testing.T) {
	press := func(m Model, k tea.KeyMsg) Model {
		mm, _ := m.Update(k)
		return mm.(Model)
	}
	m := typeText(shortcodeModel(), "ship :checkered")
	if !m.completing() || !strings.Contains(m.renderInputArea(), "🏁") {
		t.Fatal(":checkered shows no row with 🏁")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.textarea.Value(); got != "ship 🏁" || m.completing() {
		t.Fatalf("Enter: %q, row %v", got, m.completing())
	}

	// ↓ chooses the next one; Tab inserts it.
	m = typeText(shortcodeModel(), ":smi")
	second := m.complete.items[1].Emoji
	m = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.textarea.Value(); got != second {
		t.Errorf("↓ then Tab: %q, want %q", got, second)
	}

	// Esc hides it until the query changes; typing on brings it back.
	m = typeText(shortcodeModel(), ":fir")
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.completing() || m.textarea.Value() != ":fir" {
		t.Fatalf("Esc: row %v, value %q", m.completing(), m.textarea.Value())
	}
	m = typeText(m, "e")
	if !m.completing() {
		t.Error("typing after Esc didn't bring the row back")
	}

	// The row takes the input's last line, not one more.
	closed := strings.Count(typeText(shortcodeModel(), "ship").renderInputArea(), "\n")
	if open := strings.Count(typeText(shortcodeModel(), ":chec").renderInputArea(), "\n"); open != closed {
		t.Errorf("input area %d lines with the row, %d without", open+1, closed+1)
	}
}
