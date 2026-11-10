package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ctxpkg "claw-code-go/internal/context"
)

func TestModelCheck(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if r.Header.Get("Authorization") == "Bearer wrong" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"object":"list","data":[{"id":"a/listed"},{"id":"b/other"}]}`))
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name, base, key, model, status, detail string
	}{
		{"listed, base ends in /v1", srv.URL + "/kronk-only/v1", "k", "a/listed", "ok", "a/listed at"},
		{"listed, bare base", srv.URL, "", "a/listed", "ok", "a/listed at"},
		{"not listed", srv.URL + "/v1", "k", "c/missing", "FAIL", "isn't among the 2 models"},
		{"bad key", srv.URL + "/v1", "wrong", "a/listed", "FAIL", "401"},
		{"no model", srv.URL + "/v1", "k", "", "FAIL", "model isn't"},
		{"no base_url", "", "", "a/listed", "off", "no base_url"},
		{"unreachable", "http://127.0.0.1:1/v1", "", "a/listed", "FAIL", "can't reach"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := modelCheck(tc.base, tc.key, tc.model, srv.Client())
			if c.status != tc.status || !strings.Contains(c.detail, tc.detail) {
				t.Errorf("modelCheck = %s %q, want %s with %q", c.status, c.detail, tc.status, tc.detail)
			}
		})
	}

	// A server busy past the timeout - one queueing requests behind a generation - is a warning,
	// not a failure.
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)
	busy := slow.Client()
	busy.Timeout = 50 * time.Millisecond
	if c := modelCheck(slow.URL+"/v1", "", "a/listed", busy); c.status != "warn" || !strings.Contains(c.detail, "busy") {
		t.Errorf("a server busy past the timeout: %s %q, want warn", c.status, c.detail)
	}

	modelCheck(srv.URL+"/kronk-only/v1", "k", "a/listed", srv.Client())
	if gotPath != "/kronk-only/v1/models" || gotAuth != "Bearer k" {
		t.Errorf("asked %s with %q, want /kronk-only/v1/models with the key", gotPath, gotAuth)
	}
}

func TestToolsCheck(t *testing.T) {
	if c := toolsCheck(nil); c.status != "off" {
		t.Errorf("no tools: %s %q, want off", c.status, c.detail)
	}
	all := []ctxpkg.Tool{{Name: "speak", OnPath: true}, {Name: "consult", OnPath: true}}
	if c := toolsCheck(all); c.status != "ok" || c.detail != "all 2 on PATH" {
		t.Errorf("all on PATH: %s %q", c.status, c.detail)
	}
	some := []ctxpkg.Tool{{Name: "speak", OnPath: true}, {Name: "transcribe"}, {Name: "consult"}}
	if c := toolsCheck(some); c.status != "warn" || c.detail != "1 of 3 on PATH; not here: transcribe, consult" {
		t.Errorf("some missing: %s %q", c.status, c.detail)
	}
}

func TestHandoffCheck(t *testing.T) {
	dir := t.TempDir()
	if c := handoffCheck(filepath.Join(dir, "missing"), "/w"); c.status != "off" {
		t.Errorf("no condenser: %+v", c)
	}
	if c := handoffCheck(dir, "/w"); c.status != "ok" || !strings.Contains(c.detail, "none for this folder") {
		t.Errorf("none here: %+v", c)
	}
	p := filepath.Join(dir, "-w", "abc12345-x", "handoff.md")
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte("# h\n**Original size:** 2.0 MB, 9 messages (3 operator turns, 4 API responses)\n"), 0o600)
	if c := handoffCheck(dir, "/w"); c.status != "ok" || !strings.Contains(c.detail, "1 for this folder") || !strings.Contains(c.detail, "abc12345") {
		t.Errorf("one here: %+v", c)
	}
}

// doctor suggestions prints the counts and rates, split by outward, and the gate's state.
func TestSuggestionsReport(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	at := now.Format(time.RFC3339)
	var lines []string
	add := func(n int, outcome string, outward bool) {
		for i := 0; i < n; i++ {
			lines = append(lines, `{"time":"`+at+`","suggested":"s","sent":"s","outcome":"`+outcome+`","outward":`+map[bool]string{true: "true", false: "false"}[outward]+`}`)
		}
	}
	add(1, "accepted", false)
	add(3, "edited", false)
	add(25, "ignored", false)
	add(6, "ignored", true)
	lines = append(lines, "{broken")
	p := filepath.Join(t.TempDir(), "suggestions.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := suggestionsReport(&out, p, now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"suggestions: 35 total (last 30 days: 35), 1 malformed lines skipped",
		"all:      accepted 1 (2.9%)    edited 3 (8.6%)      ignored 31 (88.6%)",
		"outward:  accepted 0 (0.0%)    edited 0 (0.0%)      ignored 6 (100.0%)",
		"inward:   accepted 1 (3.4%)    edited 3 (10.3%)     ignored 25 (86.2%)",
		"outward suggestions offered: 6 of 10 entries needed to decide",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := suggestionsReport(&out, filepath.Join(t.TempDir(), "none.jsonl"), now); err != nil || out.String() != "no suggestions logged yet\n" {
		t.Errorf("no log: %q, %v", out.String(), err)
	}
}
