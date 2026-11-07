package tui

import (
	"strings"
	"testing"
)

func TestApplyThemeExplicit(t *testing.T) {
	saved := currentTheme
	t.Cleanup(func() { SetTheme(saved) })

	if name := ApplyTheme("light"); name != "light" || currentTheme != LightTheme {
		t.Errorf(`ApplyTheme("light") = %q, theme light=%v`, name, currentTheme == LightTheme)
	}
	if name := ApplyTheme("dark"); name != "dark" || currentTheme != DarkTheme {
		t.Errorf(`ApplyTheme("dark") = %q, theme dark=%v`, name, currentTheme == DarkTheme)
	}
}

func TestApplyThemeAutoDetects(t *testing.T) {
	saved := currentTheme
	t.Cleanup(func() { SetTheme(saved) })

	// "auto" and an unknown value both fall through to detection: the result
	// must be one of the two real themes and say it was detected. Which one it
	// picks depends on the terminal, so that is not asserted.
	for _, pref := range []string{"auto", "", "nonsense"} {
		name := ApplyTheme(pref)
		if !strings.Contains(name, "detected") {
			t.Errorf("ApplyTheme(%q) = %q, want a detected result", pref, name)
		}
		if currentTheme != DarkTheme && currentTheme != LightTheme {
			t.Errorf("ApplyTheme(%q) left an unknown theme", pref)
		}
	}
}
