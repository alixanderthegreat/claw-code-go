package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claw-code-go/internal/api"
	"claw-code-go/internal/permissions"
)

// diskClient plays a model that calls a tool for its first `calls` responses and then answers.
// Before answering each request it reads the session file from disk: what a crash at that moment
// would leave.
type diskClient struct {
	calls int
	path  string
	seen  []int // messages on disk when each request came in
	n     int
}

func (c *diskClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.n++
	c.seen = append(c.seen, onDisk(c.path))
	ch := make(chan api.StreamEvent, 8)
	defer close(ch)
	if c.n <= c.calls {
		raw, _ := json.Marshal(map[string]any{"command": "echo step"})
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "tool_use", ID: fmt.Sprintf("call_%d", c.n), Name: "bash"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "input_json_delta", PartialJSON: string(raw)}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "tool_use"}
	} else {
		ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: "done"}}
		ch <- api.StreamEvent{Type: api.EventContentBlockStop}
		ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
	}
	ch <- api.StreamEvent{Type: api.EventMessageStop}
	return ch, nil
}

// onDisk is how many messages the session file holds, -1 when there's no file or it isn't JSON.
func onDisk(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	var s Session
	if json.Unmarshal(b, &s) != nil {
		return -1
	}
	return len(s.Messages)
}

// TestCheckpointEveryStep: greatproject #632. The 2026-11-16 crash lost 50 minutes of Gordi
// because the session was written only on the way out. Now each step is on disk before the
// next request goes to the model: a kill at any point loses at most the step in progress.
func TestCheckpointEveryStep(t *testing.T) {
	for name, run := range entryPoints {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			client := &diskClient{calls: 3}
			loop := NewConversationLoop(&Config{Model: "m", MaxTokens: 1024, ContextWindow: 40960, SessionDir: dir}, client)
			loop.CtxAssembler = nil
			loop.PermManager = permissions.NewManager(permissions.ModeBypassPermissions, nil)
			client.path = filepath.Join(dir, loop.Session.ID+".json")
			if err := run(loop); err != nil {
				t.Fatal(err)
			}
			// before request k: the user's message, then k-1 steps of (reply, tool results)
			want := []int{1, 3, 5, 7}
			if fmt.Sprint(client.seen) != fmt.Sprint(want) {
				t.Errorf("messages on disk as each request came in: %v, want %v", client.seen, want)
			}
			if got := onDisk(client.path); got != len(loop.Session.Messages) || got != 8 {
				t.Errorf("after the turn: %d on disk, %d in memory, want 8", got, len(loop.Session.Messages))
			}
		})
	}
}

// TestNoSessionDirNoFiles: a loop without a session directory (tests, embedded uses) writes nothing.
func TestNoSessionDirNoFiles(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)
	client := &diskClient{calls: 1}
	loop := NewConversationLoop(&Config{Model: "m", MaxTokens: 1024, ContextWindow: 40960}, client)
	loop.CtxAssembler = nil
	loop.PermManager = permissions.NewManager(permissions.ModeBypassPermissions, nil)
	if err := loop.SendMessage(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("wrote %d files with no session dir", len(es))
	}
}

// TestSaveIsAtomic plants a write that fails halfway: the old session file stays whole, and no
// temp file is left behind.
func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	s := NewSession()
	s.Messages = []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "first"}}}}
	if err := SaveSession(dir, s); err != nil {
		t.Fatal(err)
	}
	old := writeFile
	defer func() { writeFile = old }()
	writeFile = func(f *os.File, data []byte) error {
		f.Write(data[:len(data)/2]) // half written, then the crash
		return errors.New("planted: disk gone")
	}
	s.Messages = append(s.Messages, api.Message{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "second"}}})
	if err := SaveSession(dir, s); err == nil || !strings.Contains(err.Error(), "planted") {
		t.Fatalf("err %v", err)
	}
	if got := onDisk(filepath.Join(dir, s.ID+".json")); got != 1 {
		t.Errorf("the old file reads as %d messages, want it whole with 1", got)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("%d files left in the session dir, want only the session", len(es))
	}
}

// TestFailedCheckpointSaidOnce: a session dir that can't be written is said once in the turn,
// and the turn still finishes.
func TestFailedCheckpointSaidOnce(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	os.WriteFile(file, nil, 0o600)
	client := &diskClient{calls: 2}
	loop := NewConversationLoop(&Config{Model: "m", MaxTokens: 1024, ContextWindow: 40960, SessionDir: file}, client)
	loop.CtxAssembler = nil
	loop.PermManager = permissions.NewManager(permissions.ModeBypassPermissions, nil)
	events := make(chan TurnEvent)
	done := make(chan error, 1)
	go func() { done <- loop.SendMessageStreaming(context.Background(), "go", events); close(events) }()
	said := 0
	for ev := range events {
		if strings.Contains(ev.Text, "session not saved") {
			said++
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if said != 1 {
		t.Errorf("the failed save was said %d times, want once", said)
	}
}
