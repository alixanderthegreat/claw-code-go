// Package serve implements the minimal dispatch surface for kata cycle 84's target
// condition: claw-code-go reachable and checkable the same way opencode is today.
//
// Deliberately NOT an in-process multi-session registry (path (a) from the recon -
// see kata 84 Test 0's own record). Every dispatch spawns claw-code-go itself as a
// fresh child process via -prompt, with its own real OS-level working directory -
// the actual gap recon found: every tool (bash, read_file, write_file, file_edit) is
// bound to the PROCESS's cwd with no session-level concept and no path-boundary
// guard at all, so many sessions sharing one process would share one directory.
// A child process per dispatch sidesteps that entirely, for free, instead of adding
// new sandboxing code.
//
// Status is read back from that process's own already-real session file
// (runtime.SaveSession/LoadSession), not a new in-memory event stream - the file IS
// the source of truth. Interrupt is a signal to the child process, reusing the
// SIGTERM handler -prompt mode already has (it saves session state and exits).
//
// Security note, same posture as kronk's own debug server: this has NO
// authentication and can spawn processes running with -permission-mode bypass
// (every tool auto-allowed, including bash). Bind it to loopback only.
package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"claw-code-go/internal/runtime"
)

// dispatchStatus is where a dispatched task is in its life.
type dispatchStatus string

const (
	statusRunning     dispatchStatus = "running"
	statusDone        dispatchStatus = "done"
	statusFailed      dispatchStatus = "failed"
	statusInterrupted dispatchStatus = "interrupted"
)

// ExitCodeInterrupted is the exit code a dispatched child reports when
// Registry.Interrupt cut it short (main.go's own -prompt signal handler exits with
// this instead of 0 - see its own comment for why 130). Exported: main.go, the
// producer, references this constant rather than main and serve each keeping their
// own copy of the same number to drift out of sync.
//
// kata 84 Test 3 found live that an interrupted dispatch and a naturally-completed
// one were indistinguishable before this existed - both exited 0, both read "done",
// and message_count was the only (inferential, not explicit) tell.
const ExitCodeInterrupted = 130

// classifyStatus is the pure decision behind what a dispatch's status reports, kept
// free of *exec.Cmd/*os.Process so it is directly unit-testable without a real
// subprocess. exited mirrors cmd.Wait() having returned at all; exitCode is
// meaningless (and ignored) while exited is false.
func classifyStatus(exited bool, exitCode int) dispatchStatus {
	if !exited {
		return statusRunning
	}
	switch exitCode {
	case 0:
		return statusDone
	case ExitCodeInterrupted:
		return statusInterrupted
	default:
		return statusFailed
	}
}

// dispatch tracks one spawned claw-code-go child process.
type dispatch struct {
	ID         string
	Dir        string // the project directory the child ran in
	SessionDir string // this dispatch's own, isolated session directory

	mu        sync.Mutex
	cmd       *exec.Cmd
	startedAt time.Time
	exited    bool
	exitCode  int // meaningless until exited is true; see cmd.ProcessState.ExitCode()
	exitErr   error
	finished  time.Time
}

// Registry tracks every dispatch this server process has spawned, in memory only -
// a real, scoped limitation of this first build: a server restart loses track of any
// dispatch it did not finish waiting on. The session files on disk (already real,
// already working) survive regardless and can be recovered by hand if that happens.
type Registry struct {
	binaryPath  string // this claw-code-go binary's own path, re-exec'd per dispatch
	sessionRoot string // parent directory; each dispatch gets its own subdirectory

	mu  sync.Mutex
	m   map[string]*dispatch
	seq int64
}

// NewRegistry creates a Registry. binaryPath is the claw-code-go executable to
// re-exec per dispatch (see os.Executable() at the call site) - sessionRoot is the
// parent directory each dispatch's own isolated session directory is created under.
func NewRegistry(binaryPath, sessionRoot string) *Registry {
	return &Registry{
		binaryPath:  binaryPath,
		sessionRoot: sessionRoot,
		m:           make(map[string]*dispatch),
	}
}

// Dispatch spawns a new claw-code-go -prompt child process for task, running in dir,
// and returns immediately with an id to poll/interrupt later - it never waits for
// the child to finish.
func (reg *Registry) Dispatch(task, dir string) (string, error) {
	reg.mu.Lock()
	reg.seq++
	id := fmt.Sprintf("disp_%d_%d", time.Now().UnixNano(), reg.seq)
	reg.mu.Unlock()

	sessDir := filepath.Join(reg.sessionRoot, id)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}

	cmd := exec.Command(reg.binaryPath, "-prompt", task, "-session-dir", sessDir, "-permission-mode", "bypass")
	cmd.Dir = dir
	// bypass allows every tool with no prompting - deliberately, this is unattended.
	// CLAW_CODE_BLOCKED_TOOLS is the one carve-out kata 84 Test 2 requires: bypass
	// mode now honors an explicit ruleset deny (internal/permissions fix, same
	// change), so this actually takes effect instead of being silently ignored the
	// way it would have been before that fix. web_fetch/web_search denied here,
	// matching opencode's own mary-dispatch posture for unattended dispatch -
	// never set for the interactive TUI, which has no reason to deny these.
	// CLAW_CODE_DISPATCH_BOUNDARY: turns on the path-boundary guard in
	// internal/tools (read_file/write_file/file_edit) - refuses any path outside
	// this dispatch's own cmd.Dir, defense in depth on top of the real OS-level
	// isolation cmd.Dir already provides. Deliberately NOT extended to bash: bash
	// takes an arbitrary shell command string, not a structured path, so the same
	// kind of check can't meaningfully cover it here - `cd elsewhere && rm file`
	// is not something a per-argument guard can catch. bash's real containment
	// stays at the OS-process/cmd.Dir level only; that gap is honest, not fixed.
	cmd.Env = append(os.Environ(),
		"CLAW_CODE_BLOCKED_TOOLS=web_fetch,web_search",
		"CLAW_CODE_DISPATCH_BOUNDARY=1",
	)

	// Real output, never silently dropped - the same spirit as Mary's own
	// coding-room ledger, just a plain log file instead of a kanban card.
	logFile, err := os.Create(filepath.Join(sessDir, "output.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	d := &dispatch{ID: id, Dir: dir, SessionDir: sessDir, startedAt: time.Now(), cmd: cmd}

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return "", fmt.Errorf("start: %w", err)
	}

	reg.mu.Lock()
	reg.m[id] = d
	reg.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()
		if logFile != nil {
			logFile.Close()
		}
		d.mu.Lock()
		d.exited = true
		d.exitErr = waitErr
		// ProcessState is populated by Wait() regardless of waitErr - ExitCode()
		// is -1 only if the process hasn't exited (impossible here, Wait() already
		// returned) or ProcessState itself is nil. Defaulting to -1 in that
		// impossible case still classifies as "failed", never a false "done".
		if cmd.ProcessState != nil {
			d.exitCode = cmd.ProcessState.ExitCode()
		} else {
			d.exitCode = -1
		}
		d.finished = time.Now()
		d.mu.Unlock()
	}()

	return id, nil
}

// DispatchStatus is what Registry.Status reports back.
type DispatchStatus struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"` // running | done | failed | interrupted
	Dir          string    `json:"dir"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	MessageCount int       `json:"message_count,omitempty"`
}

// Status reports a dispatch's current state. The session summary is read fresh from
// disk every call (runtime.ListSessionsWithMeta, already real) rather than cached,
// so it reflects the child's actual progress even while still running.
func (reg *Registry) Status(id string) (*DispatchStatus, bool) {
	reg.mu.Lock()
	d, ok := reg.m[id]
	reg.mu.Unlock()
	if !ok {
		return nil, false
	}

	d.mu.Lock()
	exited, exitCode, started, finished := d.exited, d.exitCode, d.startedAt, d.finished
	d.mu.Unlock()

	out := &DispatchStatus{
		ID:        id,
		Status:    string(classifyStatus(exited, exitCode)),
		Dir:       d.Dir,
		StartedAt: started,
	}
	if exited {
		out.FinishedAt = finished
	}

	if metas, err := runtime.ListSessionsWithMeta(d.SessionDir); err == nil && len(metas) > 0 {
		out.SessionID = metas[0].ID
		out.MessageCount = metas[0].MessageCount
	}

	return out, true
}

// Interrupt sends SIGTERM to a still-running dispatch's process - the same signal
// main.go's own -prompt mode already handles by saving session state and exiting
// cleanly. Refuses (without touching the process) once the dispatch has already
// finished, or if it never existed.
func (reg *Registry) Interrupt(id string) error {
	reg.mu.Lock()
	d, ok := reg.m[id]
	reg.mu.Unlock()
	if !ok {
		return fmt.Errorf("no such dispatch: %s", id)
	}

	d.mu.Lock()
	exited := d.exited
	proc := d.cmd.Process
	d.mu.Unlock()

	// Go's own os.Process.Signal already refuses a signal to an already-Wait()-ed
	// process (os.ErrProcessDone) - this explicit check is not load-bearing against
	// that particular case (confirmed by mutation test: removing it still passes),
	// kept for a clearer, dispatch-specific error message rather than stdlib's
	// generic one, and as a guard against any future change to how proc is read.
	if exited {
		return fmt.Errorf("dispatch %s already finished", id)
	}
	if proc == nil {
		return fmt.Errorf("dispatch %s has no running process", id)
	}
	return proc.Signal(syscall.SIGTERM)
}

// Server wraps a Registry with its HTTP handlers.
type Server struct {
	reg *Registry
}

// NewServer creates a Server over reg.
func NewServer(reg *Registry) *Server {
	return &Server{reg: reg}
}

// Mux returns the routed handler - POST /dispatch, GET /session/{id},
// POST /session/{id}/interrupt.
func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /dispatch", s.handleDispatch)
	mux.HandleFunc("GET /session/{id}", s.handleStatus)
	mux.HandleFunc("POST /session/{id}/interrupt", s.handleInterrupt)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return mux
}

type dispatchRequest struct {
	Task string `json:"task"`
	Dir  string `json:"dir"`
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	var req dispatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad request body: %w", err))
		return
	}
	if req.Task == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("'task' is required"))
		return
	}
	if req.Dir == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("'dir' is required"))
		return
	}
	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("create dir: %w", err))
		return
	}

	id, err := s.reg.Dispatch(req.Task, req.Dir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status, ok := s.reg.Status(id)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no such dispatch: %s", id))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.reg.Interrupt(id); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "interrupted"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprint(w, s.reg.Metrics())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
