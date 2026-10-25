package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestGatewayRoot(t *testing.T) {
	for in, want := range map[string]string{
		"http://192.0.2.10:8080/kronk-only/v1":  "http://192.0.2.10:8080",
		"http://192.0.2.10:8080/kronk-only/v1/": "http://192.0.2.10:8080",
		"http://localhost:8080/v1":                 "http://localhost:8080",
		"":                                         "",
	} {
		if got := GatewayRoot(in); got != want {
			t.Errorf("GatewayRoot(%q) = %q, want %q", in, got, want)
		}
	}
}

// gatewayStub collects what reaches /events, answering with status.
type gatewayStub struct {
	mu     sync.Mutex
	got    []Event
	posts  int
	status int
}

func (g *gatewayStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var batch []Event
	json.NewDecoder(r.Body).Decode(&batch)
	g.mu.Lock()
	g.posts++
	g.got = append(g.got, batch...)
	g.mu.Unlock()
	w.WriteHeader(g.status)
}

func (g *gatewayStub) count() (posts, events int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.posts, len(g.got)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("timed out")
}

// TestEventsReachTheGateway: events are sent together to <gateway>/events.
func TestEventsReachTheGateway(t *testing.T) {
	FlushGap = 20 * time.Millisecond
	stub := &gatewayStub{status: http.StatusNoContent}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	s := New(srv.URL + "/kronk-only/v1")
	s.Send("gordi_run", map[string]any{"ms": 12})
	s.Send("gordi_turn", map[string]any{"rounds": 1})
	waitFor(t, func() bool { _, n := stub.count(); return n == 2 })
	if stub.got[0].Name != "gordi_run" || stub.got[1].Fields["rounds"] != 1.0 {
		t.Errorf("got %+v", stub.got)
	}
}

// TestNoEventsEndpointTurnsSendingOff: a gateway without /events is asked once.
func TestNoEventsEndpointTurnsSendingOff(t *testing.T) {
	FlushGap = 20 * time.Millisecond
	stub := &gatewayStub{status: http.StatusNotFound}
	srv := httptest.NewServer(stub)
	defer srv.Close()
	s := New(srv.URL + "/v1")
	s.Send("gordi_run", nil)
	waitFor(t, func() bool { p, _ := stub.count(); return p == 1 })
	waitFor(t, func() bool { return s.off.Load() })
	s.Send("gordi_run", nil)
	time.Sleep(60 * time.Millisecond)
	if p, _ := stub.count(); p != 1 {
		t.Errorf("%d posts after a 404, want 1", p)
	}
	var nilSender *Sender
	nilSender.Send("gordi_run", nil) // a nil Sender drops, without a panic
}
