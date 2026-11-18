package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// rtk.go: commands through rtk (github.com/rtk-ai/rtk), which runs a command and hands back a
// summary of its output - a failing `go test -v` as the failures alone - so the model reads less
// of what it doesn't need (greatproject #648).

// rtkRewriteTimeout bounds asking rtk for its version of a command; past it, the command runs as
// the model gave it.
const rtkRewriteTimeout = 2 * time.Second

// CompressCommand is command as rtk would run it - `git status` as `rtk git status` - and true;
// or command unchanged and false, when rtk isn't on PATH, has no version of it, or doesn't answer
// in time. rtk decides which commands it has a version of, so a new rtk release covers more
// without a change here.
func CompressCommand(command string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), rtkRewriteTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rtk", "rewrite", command)
	cmd.Env = rtkEnv(os.Environ())
	out, err := cmd.Output()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return command, false // not on PATH, or killed by the timeout
		}
		code = exit.ExitCode()
	}
	// rtk rewrite's exit codes: 0 and 3 give its version on stdout (3 is "no permission rule of
	// rtk's matched" - Gordi's own permissions have already passed the command); 1 is no version;
	// 2 is a deny rule of rtk's, which isn't rtk's call to make here, so it runs unchanged.
	if code != 0 && code != 3 {
		return command, false
	}
	rewritten := strings.TrimSuffix(string(out), "\n")
	if strings.TrimSpace(rewritten) == "" || rewritten == command {
		return command, false
	}
	return rewritten, true
}

// rtkEnv is env for a command that may go through rtk: rtk keeps no copy of any output (its
// recall store would be a second record of everything the model runs, kept 90 days), sends no
// telemetry, and doesn't warn that its own Claude Code hook isn't installed.
func rtkEnv(env []string) []string {
	return append(append([]string(nil), env...),
		"RTK_RECALL=0", "RTK_TELEMETRY_DISABLED=1", "RTK_SUPPRESS_HOOK_WARNING=1")
}
