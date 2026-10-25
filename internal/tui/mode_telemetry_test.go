package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"claw-code-go/internal/permissions"
	"claw-code-go/internal/runtime"
	"claw-code-go/internal/telemetry"
)

// TestModeSwitchesAreSent: opening Gordi sends its starting permission mode, and Shift+Tab sends
// each switch - so Grafana can tell how long each mode is in force.
func TestModeSwitchesAreSent(t *testing.T) {
	telemetry.FlushGap = 20 * time.Millisecond
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []telemetry.Event
		json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		for _, e := range batch {
			if e.Name == "gordi_mode" {
				got = append(got, e.Fields)
			}
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := &runtime.Config{Model: "m", MaxTokens: 100, ContextWindow: 32768}
	loop := runtime.NewConversationLoop(cfg, &recordingClient{})
	loop.PermManager = permissions.NewManager(permissions.ModeDefault, nil)
	loop.Telemetry = telemetry.New(srv.URL + "/v1")
	m := NewModel(cfg, loop)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = mm.(Model)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("%d gordi_mode events: %v", len(got), got)
	}
	if got[0]["from"] != "start" || got[0]["to"] != "default" || got[1]["from"] != "default" || got[1]["to"] != "accept-edits" {
		t.Errorf("events: %v", got)
	}
}
