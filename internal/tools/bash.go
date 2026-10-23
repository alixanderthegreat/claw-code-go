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

const maxOutputSize = 10000

// bashTimeout bounds one command; a var so a test can shorten it.
var bashTimeout = 30 * time.Second

// BashTool returns the tool definition for the bash tool.
func BashTool() api.Tool {
	return api.Tool{
		Name:        "bash",
		Description: "Execute a bash command and return the output. Use this for running shell commands, scripts, and system operations.",
		InputSchema: api.InputSchema{
			Type: "object",
			Properties: map[string]api.Property{
				"command": {
					Type:        "string",
					Description: "The bash command to execute",
				},
			},
			Required: []string{"command"},
		},
	}
}

// ExecuteBash runs a bash command and returns combined stdout+stderr.
func ExecuteBash(input map[string]any) (string, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("bash: 'command' input is required and must be a string")
	}

	ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	// Nothing may ask for input: the TUI owns the terminal. A command that prompts (an ssh host
	// key or password, sudo, a git login) took the terminal over and hung - so it runs detached
	// from it, with git's and ssh's prompts off, and a prompt fails at once with a clear error.
	cmd.Env = noPromptEnv(os.Environ())
	detach(cmd)
	// On timeout, the whole group goes, and Run stops waiting for pipes a child still holds - the
	// 30 s limit used to kill bash alone while an ssh it started kept the output open forever.
	cmd.WaitDelay = 2 * time.Second

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()

	output := buf.String()

	// Truncate output if too long
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + "\n... [output truncated]"
	}

	if err != nil {
		// Return output + error description; the caller decides if it's a hard error
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("command timed out after %s", bashTimeout)
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
