package tui

import (
	"fmt"
	"regexp"
	"strings"

	"claw-code-go/internal/permissions"
)

// The right panel under Gordi (greatproject #437): what the header, the key hints and the status
// bar said, moved out of the chat's way when the avatar shows. It sits beside the top of the chat
// only, so the rows below copy clean; Ctrl+O's full-width view is the place to copy from.

// panelInfo is the panel's lines under the gopher's label, each cut to the panel's width.
func (m Model) panelInfo() []string {
	rule := dividerStyle.Render(strings.Repeat("─", avatarCols-2))
	var lines []string
	lines = append(lines, rule)
	if m.loop != nil {
		lines = append(lines, headerStyle.Render(shortModel(m.cfg.Model)),
			modelTagStyle.Render(m.cfg.ProviderName+" · v"+appVersion))
		if m.loop.PermManager != nil && m.loop.PermManager.Mode != permissions.ModeDefault {
			lines = append(lines, modelTagStyle.Render("⚡ "+m.loop.PermManager.Mode.String()))
		}
		if m.inputTokens > 0 || m.outputTokens > 0 {
			lines = append(lines, statusStyle.Render("in  "+shortCount(m.inputTokens)))
			if m.cachedTokens > 0 {
				lines = append(lines, statusStyle.Render("    "+shortCount(m.cachedTokens)+" cached"))
			}
			lines = append(lines, statusStyle.Render("out "+shortCount(m.outputTokens)))
		}
		if id := m.loop.Session.ID; id != "" {
			lines = append(lines, statusStyle.Render("session …"+id[max(len(id)-6, 0):]))
		}
	}
	if m.state == stateBusy {
		lines = append(lines, statusStyle.Render("Esc cancel"))
		if m.queued != "" {
			lines = append(lines, statusStyle.Render("a command is queued"))
		} else {
			lines = append(lines, statusStyle.Render("Enter: to next step"))
		}
	}
	if vs := m.voiceStatus(); vs != "" {
		lines = append(lines, statusStyle.Render(vs))
	}
	lines = append(lines, rule)
	for _, k := range [][2]string{
		{"Enter", "send"}, {"Alt+Enter", "newline"}, {"Ctrl+U", "clear"}, {"Ctrl+O", "details"},
		{"↑↓", "history"}, {"PgUp/Dn", "scroll"}, {"Shift+Tab", "mode"}, {"!cmd", "shell"},
		{"Shift+R-click", "select"},
	} {
		lines = append(lines, userLabelStyle.Render(k[0])+" "+statusStyle.Render(k[1]))
	}
	return lines
}

var modelSize = regexp.MustCompile(`(?i)(^|[-_])(\d+(?:\.\d+)?B)([-_]|$)`)

// shortModel is a model's name in the panel's width: its size and the build words that tell
// Gordi's seats apart (9B plusIQ MTP), or the name's end. The full name is /model's.
func shortModel(name string) string {
	base := name[strings.LastIndex(name, "/")+1:]
	size := ""
	if m := modelSize.FindStringSubmatch(base); m != nil {
		size = strings.ToUpper(m[2])
	}
	var tags []string
	for _, t := range []string{"plusIQ", "Jumbo", "Brill", "NEO-MAX", "MTP"} {
		if strings.Contains(base, t) {
			tags = append(tags, t)
		}
	}
	if size == "" || len(tags) == 0 {
		return base
	}
	if contains(tags, "plusIQ") { // plusIQ is a NEO-MAX build too: the one word is enough
		tags = remove(tags, "NEO-MAX")
	}
	return size + " " + strings.Join(tags, " ")
}

// shortCount is a token count in a few characters: 232k, 2.3k, 812.
func shortCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func remove(ss []string, s string) []string {
	var out []string
	for _, x := range ss {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}
