// Package telemetry sends Gordi's client events - runs, tool calls, turns - to the gateway's
// /events, which writes each as an event=gordi_* log line for Alloy to ship to Loki, as the bot's
// event=mary_* lines are. Only counts, durations and names go out, never content: gateway logs
// pass simple-obs's privacy filter whole.
//
// Sending never holds Gordi up. Events go into a buffer that a background goroutine drains; when
// the buffer is full, or the gateway doesn't answer, events are dropped. A gateway without /events
// (404) turns sending off for the rest of the process.
package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Event is one line: a name (gordi_run, gordi_tool, gordi_turn) and its fields - numbers, bools
// and short strings.
type Event struct {
	Name   string         `json:"event"`
	Fields map[string]any `json:"fields"`
}

// Sender queues events for the gateway. A nil *Sender drops everything, so callers needn't check.
type Sender struct {
	url   string
	http  *http.Client
	ch    chan Event
	flush chan chan struct{}
	off   atomic.Bool
}

const (
	buffer   = 256
	maxBatch = 32
)

// FlushGap is how long events wait to be sent with others; a var so a test can shorten it.
var FlushGap = 2 * time.Second

// New returns a Sender for the gateway behind chatBaseURL (e.g. http://core:8080/kronk-only/v1),
// or nil when telemetry is off: CLAW_TELEMETRY=off, or no base URL to derive the gateway from.
func New(chatBaseURL string) *Sender {
	if strings.EqualFold(os.Getenv("CLAW_TELEMETRY"), "off") {
		return nil
	}
	root := GatewayRoot(chatBaseURL)
	if root == "" {
		return nil
	}
	s := &Sender{url: root + "/events", http: &http.Client{Timeout: 5 * time.Second}, ch: make(chan Event, buffer), flush: make(chan chan struct{})}
	go s.run()
	return s
}

// GatewayRoot is chatBaseURL without its /v1 and /kronk-only suffixes.
func GatewayRoot(chatBaseURL string) string {
	root := strings.TrimRight(chatBaseURL, "/")
	root = strings.TrimSuffix(root, "/v1")
	root = strings.TrimSuffix(root, "/kronk-only")
	if !strings.HasPrefix(root, "http://") && !strings.HasPrefix(root, "https://") {
		return ""
	}
	return root
}

// Send queues an event, or drops it when the buffer is full or sending is off.
func (s *Sender) Send(name string, fields map[string]any) {
	if s == nil || s.off.Load() {
		return
	}
	select {
	case s.ch <- Event{Name: name, Fields: fields}:
	default:
	}
}

// Flush sends what's queued, waiting at most wait: for a process about to exit,
// whose last turn would otherwise be lost with the batch.
func (s *Sender) Flush(wait time.Duration) {
	if s == nil || s.off.Load() {
		return
	}
	done := make(chan struct{})
	select {
	case s.flush <- done:
	case <-time.After(wait):
		return
	}
	select {
	case <-done:
	case <-time.After(wait):
	}
}

func (s *Sender) run() {
	var batch []Event
	tick := time.NewTicker(FlushGap)
	defer tick.Stop()
	for {
		var done chan struct{}
		select {
		case e := <-s.ch:
			batch = append(batch, e)
			if len(batch) < maxBatch {
				continue
			}
		case <-tick.C:
			if len(batch) == 0 {
				continue
			}
		case done = <-s.flush:
			for drained := false; !drained; {
				select {
				case e := <-s.ch:
					batch = append(batch, e)
				default:
					drained = true
				}
			}
		}
		if len(batch) > 0 {
			s.post(batch)
		}
		batch = nil
		if done != nil {
			close(done)
		}
	}
}

func (s *Sender) post(batch []Event) {
	if s.off.Load() {
		return
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client", "claw-code-go")
	resp, err := s.http.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		s.off.Store(true)
	}
}
