//go:build unix

package tools

import (
	"strings"
	"testing"
	"time"
)

// TestBashCannotPrompt: a command that asks the terminal for input - as ssh does for a host key or
// password - fails at once instead of taking over the TUI.
func TestBashCannotPrompt(t *testing.T) {
	start := time.Now()
	_, err := ExecuteBash(map[string]any{"command": "read -r answer < /dev/tty && echo got $answer"})
	if err == nil {
		t.Fatal("reading the terminal should fail: there's no terminal")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s: it waited for input", time.Since(start).Round(time.Second))
	}
}

// TestBashTimeoutKillsChildren: at the time limit the whole command goes, children included - a
// child holding the output open used to keep the tool waiting long past the limit.
func TestBashTimeoutKillsChildren(t *testing.T) {
	saved := bashTimeout
	bashTimeout = time.Second
	defer func() { bashTimeout = saved }()
	start := time.Now()
	_, err := ExecuteBash(map[string]any{"command": "sleep 60 & sleep 60"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s after a 1s limit: a child kept it waiting", d.Round(time.Second))
	}
}

// TestBashPromptsOff: git and ssh are told not to prompt; a GIT_SSH_COMMAND already set is kept.
func TestBashPromptsOff(t *testing.T) {
	out, err := ExecuteBash(map[string]any{"command": "echo $GIT_TERMINAL_PROMPT; echo $GIT_SSH_COMMAND"})
	if err != nil || out != "0\nssh -o BatchMode=yes\n" {
		t.Errorf("env = %q, %v", out, err)
	}
	env := noPromptEnv([]string{"GIT_SSH_COMMAND=ssh -i key"})
	for _, kv := range env {
		if kv == "GIT_SSH_COMMAND=ssh -o BatchMode=yes" {
			t.Errorf("overrode the user's GIT_SSH_COMMAND")
		}
	}
}
