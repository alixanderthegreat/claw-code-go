package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	clawctx "claw-code-go/internal/context"
	"claw-code-go/internal/telemetry"
)

// TestCardSeesAnInstructionChange: each turn sends a gordi_card with the parts' sizes, and the
// turn after CLAUDE.md is edited says the system prompt changed - the cache breaker the blueprint
// couldn't see. /card shows the file and the prompt.
func TestCardSeesAnInstructionChange(t *testing.T) {
	telemetry.FlushGap = 20 * time.Millisecond
	var mu sync.Mutex
	var cards []telemetry.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []telemetry.Event
		json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		for _, e := range batch {
			if e.Name == "gordi_card" {
				cards = append(cards, e)
			}
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("CLAW_MEMORY_DIR", filepath.Join(dir, "memory"))
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Project\n\nUse Go."), 0o644)
	loop := newScriptedLoop(&scriptedClient{}, nil)
	loop.CtxAssembler = clawctx.NewAssembler(dir)
	loop.Telemetry = telemetry.New(srv.URL + "/v1")

	turn := func(text string) {
		events := make(chan TurnEvent, 64)
		go func() { loop.SendMessageStreaming(context.Background(), text, events); close(events) }()
		for range events {
		}
	}
	turn("one")
	later := time.Now().Add(2 * time.Second)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Project\n\nUse Go. Test everything, always."), 0o644)
	os.Chtimes(filepath.Join(dir, "CLAUDE.md"), later, later) // a new mtime, so Stable re-reads it
	turn("two")
	turn("three")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(cards)
		mu.Unlock()
		if n == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(cards) != 3 {
		t.Fatalf("%d gordi_card events, want one a turn", len(cards))
	}
	changed := []any{cards[0].Fields["system_changed"], cards[1].Fields["system_changed"], cards[2].Fields["system_changed"]}
	if changed[0] != false || changed[1] != true || changed[2] != false {
		t.Errorf("system_changed by turn: %v, want false true false", changed)
	}
	if cards[0].Fields["instruction_files"] != 1.0 || cards[0].Fields["instructions_tokens"].(float64) <= 0 {
		t.Errorf("first card: %+v", cards[0].Fields)
	}
	if h := cards[2].Fields["history_messages"].(float64); h < 5 {
		t.Errorf("history_messages on the third turn: %v", h)
	}

	text := loop.CardText()
	for _, want := range []string{"instructions", "1 file: ", "CLAUDE.md", "--- system prompt ---", "Test everything, always."} {
		if !strings.Contains(text, want) {
			t.Errorf("/card lacks %q:\n%s", want, text)
		}
	}
}
