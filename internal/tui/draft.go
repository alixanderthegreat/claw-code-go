package tui

import "fmt"

// draftLine is the activity for a tool call still being written: "writing write_file -
// path: internal/x.go · 3.2 KB". The path and size show only if the server streams the arguments;
// kronk sends them whole at the end, so there it's the name and the time.
func draftLine(name, hint string, bytes int) string {
	line := "writing " + name
	if hint != "" {
		line += " - " + truncate(hint, 80)
	}
	if bytes > 0 {
		line += " · " + formatSize(bytes)
	}
	return line
}

// progressLine is the live line for a long tool still running: "consulting deeptree - thinking ·
// ~1.8k tokens".
func progressLine(name, status string) string {
	if name == "consult" {
		return "consulting " + status
	}
	return name + " - " + status
}

func formatSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}
