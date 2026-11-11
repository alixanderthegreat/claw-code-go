package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// mascotGrid is the claw-code-go mascot, the Go gopher (Gordi), one rune per pixel. The avatar in
// the chat's corner draws him (avatar.go), two pixels to a character.
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

// mascotColors are the gopher's colors, for the splash and for the avatar beside the chat (avatar.go).
var mascotColors = map[rune]lipgloss.Color{
	'B': "39",  // body blue
	'D': "24",  // dark blue
	'L': "117", // light blue
	'W': "255", // white
}

// RenderLogo returns a styled splash block: app name + tagline. The gopher isn't in it any more:
// he's the avatar in the chat's corner (avatar.go, greatproject #434).
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

	name := nameStyle.Render("claw-code-go")
	ver := verStyle.Render(" v" + version)
	tag := tagStyle.Render("A Go port of Claude Code")
	div := divStyle.Render(strings.Repeat("─", 24))

	return fmt.Sprintf("\n  %s%s\n  %s\n  %s\n\n", name, ver, tag, div)
}
