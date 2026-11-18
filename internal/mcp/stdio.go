package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// StderrLog, when set, takes what stdio MCP servers write to stderr, a line at a time. Unset, it
// goes to the standard logger - which is the terminal, where a server's log lines would land in
// the middle of the TUI's screen (Serena's language server warns on every lookup, #649).
var StderrLog io.Writer

// StdioTransport communicates with an MCP server over stdin/stdout.
// It spawns the server as a child process and uses newline-delimited JSON.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
}

// NewStdioTransport spawns the MCP server process and returns a connected transport.
// env is a slice of "KEY=VALUE" strings appended to the current environment.
func NewStdioTransport(command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp stdio: start process: %w", err)
	}

	t := &StdioTransport{
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: bufio.NewScanner(stdoutPipe),
	}

	// Drain stderr so it doesn't block the process: to StderrLog when set, else the debug logger.
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			if w := StderrLog; w != nil {
				fmt.Fprintf(w, "%s [%s] %s\n", time.Now().Format(time.RFC3339), filepath.Base(command), scanner.Text())
				continue
			}
			log.Printf("[MCP stderr %s] %s", command, scanner.Text())
		}
	}()

	return t, nil
}

// Send writes a JSON-RPC request to the server's stdin and reads the response from stdout.
func (t *StdioTransport) Send(req Request) (Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("mcp stdio: marshal request: %w", err)
	}

	if _, err := fmt.Fprintf(t.stdin, "%s\n", data); err != nil {
		return Response{}, fmt.Errorf("mcp stdio: write request: %w", err)
	}

	if !t.stdout.Scan() {
		if err := t.stdout.Err(); err != nil {
			return Response{}, fmt.Errorf("mcp stdio: read response: %w", err)
		}
		return Response{}, fmt.Errorf("mcp stdio: server closed connection")
	}

	var resp Response
	if err := json.Unmarshal(t.stdout.Bytes(), &resp); err != nil {
		return Response{}, fmt.Errorf("mcp stdio: unmarshal response: %w", err)
	}

	return resp, nil
}

// Notify sends a JSON-RPC notification (no response expected).
func (t *StdioTransport) Notify(n Notification) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("mcp stdio: marshal notification: %w", err)
	}

	if _, err := fmt.Fprintf(t.stdin, "%s\n", data); err != nil {
		return fmt.Errorf("mcp stdio: write notification: %w", err)
	}

	return nil
}

// Close shuts down the child process.
func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}
