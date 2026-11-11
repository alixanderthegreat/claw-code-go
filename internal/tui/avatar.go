package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Gordi's avatar (greatproject #433): the Go gopher - Gophie, called Gordi here - pinned to the
// chat's top-right, his face following what Gordi is doing. He's the splash gopher (mascotGrid,
// logo.go) itself, drawn at half the size.

type avatarState int

const (
	avatarIdle     avatarState = iota // waiting for you to type: it blinks now and then
	avatarThinking                    // the model is working: eyes drift side to side
	avatarTool                        // a tool is running: eyes down, reading
	avatarSpeaking                    // a reply is being read aloud: the mouth moves
	avatarAsking                      // a permission box or a question waits on you: eyes up
)

var avatarLabels = map[avatarState]string{
	avatarIdle: "", avatarThinking: "thinking", avatarTool: "working",
	avatarSpeaking: "speaking", avatarAsking: "asking you ?",
}

const (
	avatarCols     = 20 + 2 // the splash gopher's 20 pixels, one column each, and a gutter
	avatarMinWidth = 100    // narrower than this, the chat keeps every column
	avatarTick     = 250 * time.Millisecond
	avatarBlinkGap = 16 // idle frames between blinks: one blink every 4s
)

// The splash gopher's features (mascotGrid, logo.go): each eye is 2x2 at rows 4-5, the nose is on
// row 7 in the white muzzle, and the tooth hangs under it on row 8.
var avatarEyeCols = [2]int{5, 13}

const (
	avatarEyeRow   = 4
	avatarToothRow = 8
	avatarToothCol = 9
)

type avatarTickMsg struct{}

func avatarTickCmd() tea.Cmd {
	return tea.Tick(avatarTick, func(time.Time) tea.Msg { return avatarTickMsg{} })
}

// avatarShown is whether the terminal is wide enough for the avatar beside the chat.
func (m Model) avatarShown() bool { return m.width >= avatarMinWidth }

// chatWidth is the viewport's width: the terminal's, less the avatar's column when it shows.
func (m Model) chatWidth() int {
	if m.avatarShown() {
		return m.width - avatarCols
	}
	return m.width
}

func (m Model) avatarNow() avatarState {
	switch {
	case m.state == statePermission || m.state == stateAskUser:
		return avatarAsking
	case m.voice.speaker != nil && m.voice.speaker.Speaking():
		return avatarSpeaking
	case m.state == stateBusy && m.avatarToolRunning:
		return avatarTool
	case m.state == stateBusy:
		return avatarThinking
	}
	return avatarIdle
}

// startAvatar arms the tick when the avatar shows and isn't already ticking; a narrow terminal costs
// no ticks.
func (m Model) startAvatar() (Model, tea.Cmd) {
	if !m.avatarShown() || m.avatarTicking {
		return m, nil
	}
	m.avatarTicking = true
	return m, avatarTickCmd()
}

func (m Model) onAvatarTick() (Model, tea.Cmd) {
	if !m.avatarShown() {
		m.avatarTicking = false
		return m, nil
	}
	m.avatarFrame++
	return m, avatarTickCmd()
}

// avatarGrid is the splash gopher (mascotGrid) for a state at a frame: the same gopher, his eyes
// and tooth moved.
func avatarGrid(s avatarState, frame int) [][]byte {
	g := make([][]byte, len(mascotGrid))
	for i, row := range mascotGrid {
		g[i] = []byte(row)
	}
	dx, dy := 0, 0 // looking at you
	switch s {
	case avatarThinking: // looking off to one side, then the other
		dx = []int{-1, -1, 1, 1}[frame%4]
	case avatarTool: // down at the work (not across it: the muzzle is right there)
		dy = 1
	case avatarAsking: // up at you
		dy = -1
	}
	blink := s == avatarIdle && frame > 0 && frame%avatarBlinkGap == 0
	for _, c := range avatarEyeCols {
		for y := avatarEyeRow; y < avatarEyeRow+2; y++ {
			g[y][c], g[y][c+1] = 'B', 'B'
		}
		if blink { // shut: one dark line, the lower half of the eye
			g[avatarEyeRow+1][c], g[avatarEyeRow+1][c+1] = 'D', 'D'
			continue
		}
		for y := avatarEyeRow + dy; y < avatarEyeRow+dy+2; y++ {
			g[y][c+dx], g[y][c+dx+1] = 'D', 'D'
		}
	}
	if s == avatarSpeaking && frame%2 == 1 { // the jaw drops: the mouth opens and the tooth goes down
		r, c := avatarToothRow, avatarToothCol
		g[r][c], g[r][c+1] = 'D', 'D'
		g[r+1][c], g[r+1][c+1] = 'W', 'W'
	}
	return g
}

// renderAvatar draws the gopher with half blocks, two pixels to a character - a top one in the
// foreground of "▀" and a bottom one behind it - so the splash's 20x11 gopher fits in 20 columns
// and 6 lines, pixels still square. Then the label, padded to height lines.
func renderAvatar(s avatarState, frame, height int) string {
	g := avatarGrid(s, frame)
	if len(g)%2 == 1 {
		g = append(g, []byte(strings.Repeat(".", len(g[0]))))
	}
	var lines []string
	for y := 0; y < len(g); y += 2 {
		var b strings.Builder
		b.WriteString("  ")
		for x := range g[y] {
			top, topOK := mascotColors[rune(g[y][x])]
			bot, botOK := mascotColors[rune(g[y+1][x])]
			switch {
			case topOK && botOK:
				b.WriteString(lipgloss.NewStyle().Foreground(top).Background(bot).Render("▀"))
			case topOK:
				b.WriteString(lipgloss.NewStyle().Foreground(top).Render("▀"))
			case botOK:
				b.WriteString(lipgloss.NewStyle().Foreground(bot).Render("▄"))
			default:
				b.WriteString(" ")
			}
		}
		lines = append(lines, b.String())
	}
	label := avatarLabels[s]
	lines = append(lines, "  "+statusStyle.Render(lipgloss.PlaceHorizontal(avatarCols-2, lipgloss.Center, label)))
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// withAvatar puts the avatar beside the viewport's view when there's room.
func (m Model) withAvatar(view string) string {
	if !m.avatarShown() {
		return view
	}
	// Cut to the chat's width, whatever the viewport was last sized to, so the row never runs
	// past the terminal's edge.
	view = lipgloss.NewStyle().MaxWidth(m.chatWidth()).Render(view)
	h := lipgloss.Height(view)
	return lipgloss.JoinHorizontal(lipgloss.Top, view, renderAvatar(m.avatarNow(), m.avatarFrame, h))
}
