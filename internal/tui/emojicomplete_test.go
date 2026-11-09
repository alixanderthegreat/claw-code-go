package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"claw-code-go/internal/signs"
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
	// One entry per emoji, the colour selector aside, whatever the query.
	for _, q := range []string{"chec", "heart", "sm", "star", "ar"} {
		seen := map[string]string{}
		for _, m := range emojiMatches(q, completeMax) {
			k := strings.TrimSuffix(m.Emoji, "\ufe0f")
			if c, ok := seen[k]; ok {
				t.Errorf(":%s offers %s twice (%s, %s)", q, m.Emoji, c, m.Code)
			}
			seen[k] = m.Code
		}
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

// The row offers only emoji every measure agrees are two columns, and stays short of the width,
// so it can't wrap and grow the frame (#39).
func TestEmojiRowWidth(t *testing.T) {
	var got []string
	for _, m := range emojiMatches("rob", completeMax) {
		got = append(got, m.Emoji)
		if !steadyWidth(m.Emoji) {
			t.Errorf("%s %s offered: its width isn't steady", m.Emoji, m.Code)
		}
	}
	for _, m := range emojiMatches("wav", completeMax) {
		if !steadyWidth(m.Emoji) || len([]rune(strings.TrimSuffix(m.Emoji, "\ufe0f"))) != 1 {
			t.Errorf(":wav offers %s %s, built from several code points", m.Emoji, m.Code)
		}
	}
	for _, e := range []string{"👋🏻", "🏴\U000E0067\U000E0062\U000E0077\U000E006C\U000E0073\U000E007F", "🇺🇸", "1️⃣", "👨‍🦯", "⁉️"} {
		if steadyWidth(e) || !unsteadyEmoji(e) {
			t.Errorf("%q counted steady", e)
		}
	}
	for _, e := range []string{"👋", "🤖", "🌊", "✅", "🧱"} {
		if !steadyWidth(e) {
			t.Errorf("%q counted unsteady", e)
		}
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "🤖") || strings.Contains(joined, "⁉") || strings.Contains(joined, "\u200d") {
		t.Errorf(":rob -> %q: want 🤖, and no ⁉️ or joined sequence", joined)
	}
	// Left out of the row, and since #47 not resolved either: it stays as its shortcode.
	if e, n := shortcodeAt([]rune(":interrobang:")); n != 0 {
		t.Errorf(":interrobang: resolved to %q; an unsteady emoji stays as typed", e)
	}
	for _, w := range []int{40, 80, 120} {
		m := typeText(shortcodeModel(), ":s")
		m = typeText(m, "m")
		m.width = w
		lines := strings.Split(m.renderInputArea(), "\n")
		if lw := ansi.StringWidth(lines[len(lines)-1]); lw > w-completeMargin {
			t.Errorf("width %d: the row is %d wide", w, lw)
		}
	}
}

// The twelve signs are drawn together in Ctrl+G's palette, so each must be steady too.
func TestSignsAreSteady(t *testing.T) {
	for _, s := range signs.Signs {
		if !steadyWidth(s.Emoji) {
			t.Errorf("sign %s %s isn't of steady width", s.Emoji, s.Code)
		}
	}
}
