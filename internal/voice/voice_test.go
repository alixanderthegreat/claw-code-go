package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProse: what's said aloud is the reply's sentences - not its code, tables or URLs.
func TestProse(t *testing.T) {
	md := "## Done\n\nI fixed **two** bugs in `compact.go`:\n\n" +
		"- the summary request now [reuses the cache](https://example.com/x)\n" +
		"- retention has a budget\n\n" +
		"```go\nfunc secret() {}\n```\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		"See https://example.com for more, or run `go test ./internal/runtime/ -run TestCompaction -v -count=1`.\n\n---\n"
	got := Prose(md)
	for _, want := range []string{"Done", "I fixed two bugs in compact.go:", "the summary request now reuses the cache", "retention has a budget"} {
		if !strings.Contains(got, want) {
			t.Errorf("prose lost %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"func secret", "| a |", "https://", "**", "##", "go test ./internal", "---"} {
		if strings.Contains(got, gone) {
			t.Errorf("prose still says %q:\n%s", gone, got)
		}
	}
}

// TestChunks: a short first chunk so speech starts soon, sentence-sized ones after, nothing lost,
// and no chunk over the limit even for a sentence that is.
func TestChunks(t *testing.T) {
	text := "First sentence here. Second one follows! A third? " + strings.Repeat("word ", 200) + "end.\n\nNew paragraph."
	chunks := Chunks(text, 30, 120)
	if len(chunks) < 3 || chunks[0] != "First sentence here." {
		t.Fatalf("chunks = %q", chunks)
	}
	for _, c := range chunks {
		if len(c) > 120 {
			t.Errorf("chunk of %d bytes over the 120 limit: %q", len(c), c)
		}
	}
	if strings.Join(strings.Fields(strings.Join(chunks, " ")), " ") != strings.Join(strings.Fields(text), " ") {
		t.Errorf("chunking changed the words")
	}
	if Chunks("   ", 30, 120) != nil {
		t.Errorf("blank text should be no chunks")
	}
}

// TestClientEndpoints: transcription posts the WAV as multipart with the model, speech posts the
// text as JSON, both at the chat base URL and tagged for the gateway.
func TestClientEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Client") != "claw-code-go" || r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "headers", 400)
			return
		}
		switch r.URL.Path {
		case "/kronk-only/v1/audio/transcriptions":
			f, _, err := r.FormFile("file")
			if err != nil || r.FormValue("model") != "small.en" {
				http.Error(w, "form", 400)
				return
			}
			b, _ := io.ReadAll(f)
			json.NewEncoder(w).Encode(map[string]string{"text": " heard " + string(b) + " "})
		case "/kronk-only/v1/audio/speech":
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			w.Write([]byte("WAV:" + req["input"]))
		default:
			http.Error(w, "no route "+r.URL.Path, 404)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL+"/kronk-only/v1", "k", "")

	path := filepath.Join(t.TempDir(), "a.wav")
	os.WriteFile(path, []byte("audio"), 0o600)
	text, err := c.Transcribe(context.Background(), path)
	if err != nil || text != "heard audio" {
		t.Errorf("Transcribe = %q, %v", text, err)
	}
	wav, err := c.Synthesize(context.Background(), "hello")
	if err != nil || string(wav) != "WAV:hello" {
		t.Errorf("Synthesize = %q, %v", wav, err)
	}

	bad := NewClient(srv.URL+"/nowhere", "k", "")
	if _, err := bad.Synthesize(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("a 404 should be an error naming it, got %v", err)
	}
}

// TestDropMarkers: silence is no text; real speech keeps a marker it happens to contain.
func TestDropMarkers(t *testing.T) {
	for in, want := range map[string]string{
		"[BLANK_AUDIO]":                 "",
		" (wind blowing) [MUSIC] ":      "",
		"fix the bug (the compact one)": "fix the bug (the compact one)",
	} {
		if got := dropMarkers(in); got != want {
			t.Errorf("dropMarkers(%q) = %q, want %q", in, got, want)
		}
	}
}
