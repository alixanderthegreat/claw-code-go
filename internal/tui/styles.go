package tui

import "github.com/charmbracelet/lipgloss"

// All TUI styles are derived from currentTheme.
// Call rebuildStyles (via SetTheme) to refresh after a theme switch.
var (
	headerStyle          lipgloss.Style
	modelTagStyle        lipgloss.Style
	userLabelStyle       lipgloss.Style
	assistantLabelStyle  lipgloss.Style
	toolRunningStyle     lipgloss.Style
	toolDoneStyle        lipgloss.Style
	toolFailedStyle      lipgloss.Style
	thinkingHeaderStyle  lipgloss.Style
	thinkingStyle        lipgloss.Style
	statusStyle          lipgloss.Style
	warnStyle            lipgloss.Style
	errorStyle           lipgloss.Style
	helpBoxStyle         lipgloss.Style
	dividerStyle         lipgloss.Style
	inputPromptStyle     lipgloss.Style
	pickerHeaderStyle    lipgloss.Style
	selectedModelStyle   lipgloss.Style
	unselectedModelStyle lipgloss.Style
)

// init seeds styles from the default theme before the first render.
func init() { rebuildStyles(currentTheme) }

// renderBlock styles text and appends a plain, unstyled blank-line separator.
//
// Embedding that separator inside the styled Render() call instead (e.g.
// style.Render(text+"\n\n")) is unsafe: lipgloss treats a multi-line Render()
// argument as one block, padding every line to the width of the widest line
// in that same call and dropping the final trailing newline. Whatever gets
// concatenated next then silently glues onto the end of a padded blank line
// instead of starting a fresh one — visible as stray leading whitespace on
// the next real line. Always build separator-terminated styled text through
// this helper instead of embedding "\n" inside a Render() argument.
func renderBlock(style lipgloss.Style, text string) string {
	return style.Render(text) + "\n\n"
}

// rebuildStyles recreates all styles from the given theme tokens.
func rebuildStyles(t Theme) {
	headerStyle = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	modelTagStyle = lipgloss.NewStyle().
		Foreground(t.Muted)

	userLabelStyle = lipgloss.NewStyle().
		Foreground(t.UserLabel).
		Bold(true)

	assistantLabelStyle = lipgloss.NewStyle().
		Foreground(t.AssistantLabel).
		Bold(true)

	toolRunningStyle = lipgloss.NewStyle().
		Foreground(t.ToolRunning).
		Italic(true)

	toolDoneStyle = lipgloss.NewStyle().
		Foreground(t.ToolDone)

	toolFailedStyle = lipgloss.NewStyle().
		Foreground(t.ToolFailed).
		Bold(true)

	thinkingHeaderStyle = lipgloss.NewStyle().
		Foreground(t.Muted).
		Bold(true)

	thinkingStyle = lipgloss.NewStyle().
		Foreground(t.Muted).
		Italic(true)

	statusStyle = lipgloss.NewStyle().
		Foreground(t.Muted)

	warnStyle = lipgloss.NewStyle().
		Foreground(t.Warning)

	errorStyle = lipgloss.NewStyle().
		Foreground(t.Error)

	helpBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Secondary).
		Padding(1, 2)

	dividerStyle = lipgloss.NewStyle().
		Foreground(t.Subtle)

	inputPromptStyle = lipgloss.NewStyle().
		Foreground(t.InputPrompt).
		Bold(true)

	pickerHeaderStyle = lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Padding(0, 1)

	selectedModelStyle = lipgloss.NewStyle().
		Foreground(t.SelectedItem).
		Bold(true)

	unselectedModelStyle = lipgloss.NewStyle().
		Foreground(t.UnselectedItem)
}
