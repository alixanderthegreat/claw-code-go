package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A table Glamour draws is as wide as its widest cells (its word wrap is off, #6). Wider than the
// screen, wrapping it line by line spilled the rule onto a line of its own and split each row at
// its bars (#51). fitTables lays such a table out again to the width: the widest columns narrowed
// first, each cell's text wrapped inside its column, the rule redrawn to match.

// tableMinCol is the narrowest a column is squeezed to, unless its widest cell is narrower.
const tableMinCol = 6

// isTableRule says a line, without its styles, is a table's rule: dashes crossed by ┼.
func isTableRule(plain string) bool {
	t := strings.TrimSpace(plain)
	return strings.Contains(t, "┼") && strings.Trim(t, "─┼") == ""
}

// fitTables finds each table in Glamour's lines that is wider than width and lays it out again.
// Other lines are left as they are.
func fitTables(lines []string, width int) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		// A table is a header row, its rule, and the rows after it, each with the column bars.
		if i+1 >= len(lines) || !isTableRule(ansi.Strip(lines[i+1])) || !strings.Contains(ansi.Strip(lines[i]), "│") {
			out = append(out, lines[i])
			continue
		}
		end := i + 2
		for end < len(lines) && strings.Count(ansi.Strip(lines[end]), "│") == strings.Count(ansi.Strip(lines[i+1]), "┼") {
			end++
		}
		block := lines[i:end]
		wide := false
		for _, l := range block {
			wide = wide || ansi.StringWidth(l) > width
		}
		if wide {
			block = fitTable(block, width)
		}
		out = append(out, block...)
		i = end - 1
	}
	return out
}

// fitTable lays out one table (header, rule, rows) to width. Its cells lose their styles: a table
// too wide to fit is read for its words. If even the narrowest columns don't fit, its lines are
// cut at the edge instead of wrapped.
func fitTable(block []string, width int) []string {
	rule := ansi.Strip(block[1])
	indent := len(rule) - len(strings.TrimLeft(rule, " "))
	var rows [][]string
	for i, l := range block {
		if i == 1 {
			continue
		}
		cells := strings.Split(ansi.Strip(l)[min(indent, len(ansi.Strip(l))):], "│")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		rows = append(rows, cells)
	}
	n := len(rows[0])
	cols := make([]int, n)
	for _, r := range rows {
		for j := 0; j < n && j < len(r); j++ {
			cols[j] = max(cols[j], ansi.StringWidth(r[j]))
		}
	}
	// Each cell has a space on either side, and a bar between columns: 3n-1 columns of frame.
	avail := width - indent - (3*n - 1)
	floor := func(j int) int { return min(cols[j], tableMinCol) }
	total := 0
	for _, w := range cols {
		total += w
	}
	for total > avail {
		widest := -1
		for j := range cols {
			if cols[j] > floor(j) && (widest < 0 || cols[j] > cols[widest]) {
				widest = j
			}
		}
		if widest < 0 {
			cut := make([]string, len(block))
			for i, l := range block {
				cut[i] = ansi.Truncate(l, width, "…")
			}
			return cut
		}
		cols[widest]--
		total--
	}

	pad := strings.Repeat(" ", indent)
	var out []string
	for i, r := range rows {
		wrapped := make([][]string, n)
		height := 1
		for j := range n {
			cell := ""
			if j < len(r) {
				cell = r[j]
			}
			wrapped[j] = strings.Split(ansi.Wrap(cell, cols[j], ""), "\n")
			height = max(height, len(wrapped[j]))
		}
		for k := range height {
			parts := make([]string, n)
			for j := range n {
				s := ""
				if k < len(wrapped[j]) {
					s = strings.TrimRight(wrapped[j][k], " ")
				}
				parts[j] = s + strings.Repeat(" ", cols[j]-ansi.StringWidth(s))
			}
			out = append(out, pad+" "+strings.Join(parts, " │ ")+" ")
		}
		if i == 0 {
			segs := make([]string, n)
			for j, w := range cols {
				segs[j] = strings.Repeat("─", w+2)
			}
			out = append(out, pad+strings.Join(segs, "┼"))
		}
	}
	return out
}
