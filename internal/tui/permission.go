package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// permission.go: the box that asks before a tool runs. It spans the screen and shows all of what's
// being allowed - the whole command, the whole edit - and when that's taller than the screen it
// scrolls inside the box, with the question and the keys always in view. It used to be a 72-column
// box in the bottom-left corner fed a 300-character summary, so a long command was cut twice over
// and an edit showed only its path (2026-10-24).

// permMinConversation is how much of the conversation stays in view above the box.
const permMinConversation = 3

// permFixedLines are the box's lines around the body: border top and bottom, the question and the
// blank under it, the blank above the keys and the keys.
const permFixedLines = 6

// permInnerWidth is the text width inside the box: the screen less border and padding.
func (m Model) permInnerWidth() int { return max(m.width-4, 20) }

// permBodyLines is what's being allowed, as screen lines at the box's width.
func (m Model) permBodyLines() []string {
	width := m.permInnerWidth()
	var lines []string
	add := func(style *lipgloss.Style, prefix, text string) {
		text = strings.ReplaceAll(strings.TrimRight(text, "\n"), "\t", "    ")
		for _, l := range strings.Split(text, "\n") {
			w := strings.Split(strings.TrimRight(wrapAtSpaces(prefix+l, width), " \n"), "\n")
			for _, wl := range w {
				wl = strings.TrimRight(wl, " ")
				if style != nil {
					wl = style.Render(wl)
				}
				lines = append(lines, wl)
			}
		}
	}
	str := func(key string) (string, bool) {
		v, ok := m.permArgs[key].(string)
		return v, ok
	}

	switch command, isBash := str("command"); {
	case m.permArgs == nil:
		add(nil, "", m.permToolInput)
	case m.permToolName == "bash" && isBash:
		add(nil, "", command)
	case m.permToolName == "file_edit":
		path, _ := str("file_path")
		add(&userLabelStyle, "", path)
		if all, _ := m.permArgs["replace_all"].(bool); all {
			add(&statusStyle, "", "(every occurrence)")
		}
		old, _ := str("old_string")
		updated, _ := str("new_string")
		add(&errorStyle, "- ", old)
		add(&toolDoneStyle, "+ ", updated)
	case m.permToolName == "write_file":
		path, _ := str("path")
		content, _ := str("content")
		add(&userLabelStyle, "", path)
		add(&statusStyle, "", fmt.Sprintf("%d lines:", strings.Count(strings.TrimRight(content, "\n"), "\n")+1))
		add(nil, "", content)
	default:
		keys := make([]string, 0, len(m.permArgs))
		for k := range m.permArgs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, ok := m.permArgs[k].(string)
			if !ok {
				raw, _ := json.Marshal(m.permArgs[k])
				v = string(raw)
			}
			add(nil, k+": ", v)
		}
	}
	if len(lines) == 0 {
		lines = []string{statusStyle.Render("(no input)")}
	}
	return lines
}

// permLayout is the body's lines and how many of them fit: the box takes what the screen has, less
// a few lines of the conversation, and when the body doesn't fit a line says where you are in it.
func (m Model) permLayout() (lines []string, shown int, overflow bool) {
	lines = m.permBodyLines()
	room := max(m.height-permMinConversation-permFixedLines, 1)
	if len(lines) <= room {
		return lines, len(lines), false
	}
	return lines, max(room-1, 1), true // one line for "lines 1-20 of 84"
}

// permScrollBy moves the body by n lines, kept in range.
func (m Model) permScrollBy(n int) Model {
	lines, shown, _ := m.permLayout()
	m.permScroll = min(max(m.permScroll+n, 0), max(len(lines)-shown, 0))
	return m
}

func (m Model) viewPermission() string {
	lines, shown, overflow := m.permLayout()
	top := min(m.permScroll, max(len(lines)-shown, 0))
	body := lines[top : top+shown]

	parts := []string{
		headerStyle.Render("Allow ") + userLabelStyle.Render(m.permToolName) + headerStyle.Render("?"),
		"",
	}
	parts = append(parts, body...)
	if overflow {
		parts = append(parts, statusStyle.Render(fmt.Sprintf("lines %d-%d of %d · ↑/↓ PgUp/PgDn Home/End to scroll",
			top+1, top+shown, len(lines))))
	}
	keys := "[y]es once  [a]lways  [n]o / Esc: stop the turn"
	parts = append(parts, "", statusStyle.Render(keys))
	return permBoxStyle.Width(max(m.width-2, 20)).Render(strings.Join(parts, "\n"))
}

func (m Model) viewPermissionOverlay() string {
	// The box is as tall as what's being allowed, up to the screen less a few lines; the
	// conversation gets the rest.
	box := m.viewPermission()
	vp := m.viewport
	vp.Height = max(m.height-lipgloss.Height(box), 1)
	return lipgloss.JoinVertical(lipgloss.Left, vp.View(), box)
}
