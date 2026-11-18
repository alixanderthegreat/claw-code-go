package main

import (
	"os"
	"path/filepath"
)

// maxMCPLog is how big mcp.log may grow before a start begins it again: it's for looking at why a
// server misbehaved today, not a record.
const maxMCPLog = 10 << 20

// openMCPLog opens path for MCP servers' stderr (mcp.StderrLog): appending, readable only by you,
// and started afresh when it's past maxMCPLog.
func openMCPLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxMCPLog {
		flags |= os.O_TRUNC
	}
	return os.OpenFile(path, flags, 0o600)
}
