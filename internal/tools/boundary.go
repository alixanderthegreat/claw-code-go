package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dispatchBoundaryEnv is set by internal/serve for every dispatched child process,
// and ONLY there - never for the interactive TUI, which has no reason for any
// directory it might legitimately want (a config file elsewhere, a shared
// document) to suddenly start refusing. Its value doesn't matter, only its
// presence; the boundary itself is always the process's own current directory,
// which the dispatcher already set correctly via cmd.Dir - one source of truth,
// not a second copy of the same path that could drift from it.
const dispatchBoundaryEnv = "CLAW_CODE_DISPATCH_BOUNDARY"

// requireWithinDispatchBoundary refuses any path outside the dispatch's own working
// directory when running under a dispatch. Real OS-level cwd isolation (cmd.Dir,
// one process per dispatch) is the primary boundary - this is defense in depth on
// top of it: recon for kata 84 Test 2 found there was no code-level check at all
// before this existed, and an absolute path went wherever it pointed regardless of
// what cmd.Dir was set to. A no-op outside a dispatch (the env var unset).
func requireWithinDispatchBoundary(path string) error {
	if os.Getenv(dispatchBoundaryEnv) == "" {
		return nil
	}

	boundary, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("dispatch boundary: resolve working directory: %w", err)
	}
	absBoundary, err := filepath.Abs(boundary)
	if err != nil {
		return fmt.Errorf("dispatch boundary: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("dispatch boundary: resolve %q: %w", path, err)
	}

	rel, err := filepath.Rel(absBoundary, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside this dispatch's assigned directory (%s) - refused", path, absBoundary)
	}
	return nil
}
