package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines the named color tokens used throughout the TUI.
// All lipgloss styles are derived from the active theme.
type Theme struct {
	Primary        lipgloss.Color // app accent (header, spinner)
	Secondary      lipgloss.Color // secondary accent (overlay borders)
	Success        lipgloss.Color // positive outcomes
	Warning        lipgloss.Color // advisory messages
	Error          lipgloss.Color // error messages
	Muted          lipgloss.Color // de-emphasized text (status bar)
	Subtle         lipgloss.Color // very subtle (dividers)
	UserLabel      lipgloss.Color // "You:" label
	AssistantLabel lipgloss.Color // "Claude:" label
	ToolRunning    lipgloss.Color // in-progress tool indicator
	ToolDone       lipgloss.Color // completed tool indicator
	ToolFailed     lipgloss.Color // failed tool indicator
	InputPrompt    lipgloss.Color // ">" prompt glyph
	SelectedItem   lipgloss.Color // highlighted item in pickers
	UnselectedItem lipgloss.Color // unselected item in pickers
}

// DarkTheme is the default color scheme for dark-background terminals.
var DarkTheme = Theme{
	Primary:        "205",
	Secondary:      "62",
	Success:        "82",
	Warning:        "214",
	Error:          "196",
	Muted:          "240",
	Subtle:         "238",
	UserLabel:      "33",
	AssistantLabel: "82",
	ToolRunning:    "214",
	ToolDone:       "240",
	ToolFailed:     "196",
	InputPrompt:    "33",
	SelectedItem:   "170",
	UnselectedItem: "252",
}

// LightTheme is a color scheme for light-background terminals.
var LightTheme = Theme{
	Primary:        "125",
	Secondary:      "61",
	Success:        "28",
	Warning:        "130",
	Error:          "160",
	Muted:          "246",
	Subtle:         "250",
	UserLabel:      "25",
	AssistantLabel: "28",
	ToolRunning:    "130",
	ToolDone:       "246",
	ToolFailed:     "160",
	InputPrompt:    "25",
	SelectedItem:   "125",
	UnselectedItem: "236",
}

// currentTheme is the active theme; defaults to DarkTheme.
var currentTheme = DarkTheme

// SetTheme sets the active theme and rebuilds all derived styles.
func SetTheme(t Theme) {
	currentTheme = t
	rebuildStyles(t)
}

// ApplyTheme sets the theme from a preference: "light" or "dark" force a
// scheme; anything else ("auto" or empty) detects the terminal's background
// and matches it, so Gordi no longer starts dark on a light terminal. A name
// is returned for display.
//
// Detection (lipgloss.HasDarkBackground) queries the terminal, so call this
// before the bubbletea program takes over the screen. When it can't tell (a
// pipe, an unresponsive terminal) lipgloss reports a dark background, which
// keeps the long-standing default.
func ApplyTheme(pref string) string {
	switch pref {
	case "light":
		SetTheme(LightTheme)
		return "light"
	case "dark":
		SetTheme(DarkTheme)
		return "dark"
	default:
		if lipgloss.HasDarkBackground() {
			SetTheme(DarkTheme)
			return "dark (detected)"
		}
		SetTheme(LightTheme)
		return "light (detected)"
	}
}
