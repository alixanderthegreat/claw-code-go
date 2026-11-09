package context

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// SystemInfo is the per-turn environment line: the absolute working directory (never ~, which
// file tools don't expand) and the clock.
func SystemInfo(workDir string) string {
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	}
	return "cwd: " + workDir + " | " + time.Now().Format("2006-01-02 15:04:05 MST")
}

// Platform is the part of the environment fixed for the session - OS, architecture, shell - for
// the system prompt. Built only from values that don't change while the process runs, so it is
// byte-identical every turn.
func Platform() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "unknown"
	}
	return "OS: " + runtime.GOOS + "/" + runtime.GOARCH + " | shell: " + shell
}
