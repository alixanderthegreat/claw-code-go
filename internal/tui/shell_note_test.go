package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/api"
	"claw-code-go/internal/runtime"
)

// recordingClient answers "ok" and keeps what it was sent.
type recordingClient struct {
	mu   sync.Mutex
	reqs []api.CreateMessageRequest
}

func (c *recordingClient) StreamResponse(_ context.Context, req api.CreateMessageRequest) (<-chan api.StreamEvent, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	c.mu.Unlock()
	ch := make(chan api.StreamEvent, 4)
	ch <- api.StreamEvent{Type: api.EventContentBlockStart, ContentBlock: api.ContentBlockInfo{Type: "text"}}
	ch <- api.StreamEvent{Type: api.EventContentBlockDelta, Delta: api.Delta{Type: "text_delta", Text: "ok"}}
	ch <- api.StreamEvent{Type: api.EventMessageDelta, StopReason: "end_turn"}
	close(ch)
	return ch, nil
}

// TestShellOutputGetsAReply: "! cmd" in the TUI sends the command's output to
// the model on its own, and the model answers it, as Claude Code does. It used
// to wait for the user's next message (and before that, with project context
// on - as the TUI always has - the note was built and then dropped).
func TestShellOutputGetsAReply(t *testing.T) {
	client := &recordingClient{}
	cfg := &runtime.Config{Model: "m", MaxTokens: 100, ContextWindow: 32768}
	loop := runtime.NewConversationLoop(cfg, client)
	m := NewModel(cfg, loop)
	m.width, m.height = 100, 30

	mm, cmd := m.submitText("!echo 24 The Porcaro Halloween Party")
	m = mm.(Model)
	for _, msg := range drain(cmd) { // run the shell command and deliver its result
		mm, _ = m.Update(msg)
		m = mm.(Model)
	}
	if m.state != stateBusy {
		t.Fatalf("after the command: state %v, want a turn under way", m.state)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client.mu.Lock()
		n := len(client.reqs)
		client.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.reqs) == 0 {
		t.Fatal("no request sent: the command got no reply")
	}
	msgs := client.reqs[0].Messages
	last := msgs[len(msgs)-1].Content[0].Text
	if !strings.HasSuffix(last, "$ echo 24 The Porcaro Halloween Party\n24 The Porcaro Halloween Party") {
		t.Errorf("the model was sent:\n%s", last)
	}
}

// drain runs cmd and any batch inside it, returning the messages they produce (spinner ticks aside).
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c == nil {
				continue
			}
			if inner := c(); inner != nil {
				if _, isTick := inner.(interface{ Tick() }); !isTick {
					out = append(out, inner)
				}
			}
		}
		return out
	}
	return []tea.Msg{msg}
}
