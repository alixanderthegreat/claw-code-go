package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Gordi's avatar (greatproject #433): the Go gopher - Gophie, called Gordi here - pinned to the
// chat's top-right, its face following what Gordi is doing. Drawn in colored cells like the splash
// gopher (logo.go), which stay aligned in any font where block glyphs don't.

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

// avatarFace is the gopher at rest. Legend as in logo.go, plus 'T' tan (the snout); the eyes'
// pupils and the mouth are drawn per frame.
var avatarFace = []string{
	".L......L.",
	"BBBBBBBBBB",
	"BWWWBBWWWB",
	"BWWWBBWWWB",
	"BBBBTTBBBB",
	"BBBTDDTBBB",
	"BBBBWWBBBB",
	".BBBBBBBB.",
}

const (
	avatarCols     = 2*10 + 2 // 10 cells of 2 columns, and a gutter
	avatarMinWidth = 100      // narrower than this, the chat keeps every column
	avatarTick     = 250 * time.Millisecond
	avatarBlinkGap = 16 // idle frames between blinks: one blink every 4s
	eyeRow         = 2  // the eyes' top row; each eye is 3 wide and 2 tall
	mouthRow       = 6
)

var eyeCols = [2]int{1, 6} // each eye's left column

var avatarCells = map[byte]lipgloss.Style{
	'B': lipgloss.NewStyle().Background(lipgloss.Color("39")),  // body blue
	'D': lipgloss.NewStyle().Background(lipgloss.Color("24")),  // dark blue: pupils, nose
	'L': lipgloss.NewStyle().Background(lipgloss.Color("117")), // light blue: ears
	'W': lipgloss.NewStyle().Background(lipgloss.Color("255")), // white: eyes, teeth
	'T': lipgloss.NewStyle().Background(lipgloss.Color("223")), // tan: the snout
}

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

// avatarGrid is the face for a state at a frame.
func avatarGrid(s avatarState, frame int) [][]byte {
	g := make([][]byte, len(avatarFace))
	for i, row := range avatarFace {
		g[i] = []byte(row)
	}
	dx, dy := 1, 1 // looking at you
	switch s {
	case avatarThinking:
		dx, dy = []int{0, 0, 2, 2}[frame%4], 0 // left, then right: never up and centered, which is asking
	case avatarTool:
		dx, dy = frame/2%2, 1
	case avatarAsking:
		dx, dy = 1, 0
	case avatarIdle:
		if frame%avatarBlinkGap == 0 && frame > 0 {
			for _, c := range eyeCols { // closed: the lids down, a dark line
				for x := c; x < c+3; x++ {
					g[eyeRow][x], g[eyeRow+1][x] = 'B', 'D'
				}
			}
			return g
		}
	}
	for _, c := range eyeCols {
		g[eyeRow+dy][c+dx] = 'D'
	}
	if s == avatarSpeaking && frame%2 == 1 {
		copy(g[mouthRow], "BBBDWWDBBB") // the mouth open around the teeth
	}
	return g
}

// renderAvatar draws the face and the label under it, height lines tall (padded), for the column
// beside the chat.
func renderAvatar(s avatarState, frame, height int) string {
	var lines []string
	for _, row := range avatarGrid(s, frame) {
		var b strings.Builder
		b.WriteString("  ")
		for _, c := range row {
			if style, ok := avatarCells[c]; ok {
				b.WriteString(style.Render("  "))
			} else {
				b.WriteString("  ")
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
