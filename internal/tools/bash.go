package tools

import (
	"bytes"
	"claw-code-go/internal/api"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// maxOutputSize caps what one command returns to the model. The old 10 KB cut off a consult
// answer or a long file mid-way; 50 KB (~12k tokens) fits a full consult reply while still
// bounding a runaway dump against the context window.
const maxOutputSize = 50000

// bashTimeout is the default bound on one command; a var so a test can shorten it. A command may
// ask for more via the "timeout" input, up to maxBashTimeout. The old 30 s default killed anything
// that waited - a `consult` answer, a race test, an `imagegen wait` - so it is a generous default.
var bashTimeout = 120 * time.Second

// maxBashTimeout caps what a command may request, so a hung one still dies instead of blocking the
// agent forever.
const maxBashTimeout = 10 * time.Minute

// BashTool returns the tool definition for the bash tool.
func BashTool() api.Tool {
	return api.Tool{
		Name:        "bash",
		Description: "Execute a bash command and return the output. Use this for running shell commands, scripts, and system operations. Long jobs (a consult, a build, a race test) need their own timeout: pass `timeout` in seconds, up to 600.",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"command": {
					Type:        "string",
					Description: "The bash command to execute",
				},
				"timeout": {
					Type:        "integer",
					Description: "How many seconds to allow before the command is killed. Default 120, maximum 600. Raise it for commands that wait (consult, a long test or build).",
				},
			},
			Required: []string{"command"},
		},
	}
}

// commandTimeout picks the timeout for one call: the "timeout" input in seconds when given and
// positive, clamped to maxBashTimeout, else the default bashTimeout.
func commandTimeout(input map[string]any) time.Duration {
	raw, ok := input["timeout"]
	if !ok {
		return bashTimeout
	}
	var secs float64
	switch v := raw.(type) {
	case float64: // JSON numbers decode as float64
		secs = v
	case int:
		secs = float64(v)
	case int64:
		secs = float64(v)
	default:
		return bashTimeout
	}
	if secs <= 0 {
		return bashTimeout
	}
	d := time.Duration(secs * float64(time.Second))
	if d > maxBashTimeout {
		return maxBashTimeout
	}
	return d
}

// ExecuteBash runs a bash command and returns combined stdout+stderr.
func ExecuteBash(input map[string]any) (string, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("bash: 'command' input is required and must be a string")
	}

	timeout := commandTimeout(input)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	// Nothing may ask for input: the TUI owns the terminal. A command that prompts (an ssh host
	// key or password, sudo, a git login) took the terminal over and hung - so it runs detached
	// from it, with git's and ssh's prompts off, and a prompt fails at once with a clear error.
	cmd.Env = noPromptEnv(os.Environ())
	detach(cmd)
	// On timeout, the whole group goes, and Run stops waiting for pipes a child still holds - the
	// timeout used to kill bash alone while an ssh it started kept the output open forever.
	cmd.WaitDelay = 2 * time.Second

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()

	output := buf.String()

	// Truncate output if too long, and say how to get the rest.
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + fmt.Sprintf("\n... [output truncated at %d characters; redirect to a file and read the part you need, e.g. `cmd > /tmp/out; sed -n 1,200p /tmp/out`]", maxOutputSize)
	}

	if err != nil {
		// Return output + error description; the caller decides if it's a hard error
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("command timed out after %s (pass a larger `timeout`, up to %s, for a command that waits)", timeout, maxBashTimeout)
		}
		// For non-zero exit codes, return output with error appended
		return output, fmt.Errorf("command exited with error: %v", err)
	}

	return output, nil
}

// noPromptEnv is env with git's and ssh's interactive prompts turned off, keeping a GIT_SSH_COMMAND
// the user already set.
func noPromptEnv(env []string) []string {
	out := append([]string(nil), env...)
	hasSSHCommand := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			hasSSHCommand = true
		}
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0", "SSH_ASKPASS_REQUIRE=never")
	if !hasSSHCommand {
		out = append(out, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return out
}
