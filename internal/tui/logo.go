package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// mascotGrid is a pixel-art grid of the claw-code-go cat mascot: one rune per
// pixel. Each pixel renders as a 2-character-wide colored cell (via
// background color, not glyphs), so it reads as a roughly square pixel and
// stays perfectly aligned regardless of the terminal font — unicode
// box-drawing characters (█▐▌▀▄) render at inconsistent widths across fonts
// and were the cause of the old mascot's misaligned, unreadable look.
//
// Legend: '.' transparent, 'B' body blue, 'D' dark blue (eyes/nose/outline),
// 'L' light blue (ear highlight), 'W' white (muzzle/teeth).
var mascotGrid = []string{
	"..BB............BB..",
	"..LL............LL..",
	".BBBDBBBBBBBBBBDBBB.",
	".BBBBBBBBBBBBBBBBBB.",
	".BBBBDDBBBBBBDDBBBB.",
	".BBBBDDBBBBBBDDBBBB.",
	".BBBBBBWWWWWWBBBBBB.",
	".BBBBBBWWDDWWBBBBBB.",
	".BBBBBBBBWWBBBBBBBB.",
	".BBBBBBBBBBBBBBBBBB.",
	".BBB..BBB..BBB..BBB.",
}

// renderMascot renders mascotGrid as colored 2-space cells, one grid row per
// terminal line.
func renderMascot() string {
	cell := map[rune]lipgloss.Style{
		'B': lipgloss.NewStyle().Background(lipgloss.Color("39")),  // body blue
		'D': lipgloss.NewStyle().Background(lipgloss.Color("24")),  // dark blue
		'L': lipgloss.NewStyle().Background(lipgloss.Color("117")), // light blue
		'W': lipgloss.NewStyle().Background(lipgloss.Color("255")), // white
	}

	lines := make([]string, len(mascotGrid))
	for i, row := range mascotGrid {
		var b strings.Builder
		for _, c := range row {
			if style, ok := cell[c]; ok {
				b.WriteString(style.Render("  "))
			} else {
				b.WriteString("  ")
			}
		}
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

// RenderLogo returns a styled splash block: pixel-art mascot + app name + tagline.
// Injected into the viewport on startup; scrolls away naturally as the
// conversation grows.
func RenderLogo(version string) string {
	bodyColor := lipgloss.Color("39")  // matches mascot body blue
	dimColor := lipgloss.Color("240")  // muted grey for subtitle
	divColor := lipgloss.Color("238")  // very subtle divider

	nameStyle := lipgloss.NewStyle().Foreground(bodyColor).Bold(true)
	verStyle := lipgloss.NewStyle().Foreground(dimColor)
	tagStyle := lipgloss.NewStyle().Foreground(dimColor).Italic(true)
	divStyle := lipgloss.NewStyle().Foreground(divColor)

	cat := renderMascot()
	name := nameStyle.Render("claw-code-go")
	ver := verStyle.Render(" v" + version)
	tag := tagStyle.Render("A Go port of Claude Code")
	div := divStyle.Render(strings.Repeat("─", 24))

	return fmt.Sprintf("%s\n\n  %s%s\n  %s\n  %s\n\n", cat, name, ver, tag, div)
}
