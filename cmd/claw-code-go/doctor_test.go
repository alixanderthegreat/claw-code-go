package main

import (
	"net/http"
	"net/http/httptest"
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
