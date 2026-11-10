// Package voice is speech in and out for the TUI: record from the mic, transcribe through the
// server's OpenAI-compatible /audio/transcriptions, and speak replies through /audio/speech.
//
// Both endpoints are reached at the same base URL as chat. Behind simple-bot's gateway that's
// kronk's Whisper for transcription and the bot's Supertonic - Mary's voice - for speech.
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultSTTModel is the transcription model asked for when none is configured: the Whisper
// kronk serves.
const DefaultSTTModel = "small.en"

// Client talks to a server's audio endpoints.
type Client struct {
	BaseURL  string // as for chat, e.g. http://core:8080/kronk-only/v1
	APIKey   string
	STTModel string
	// Voice is the speech service's voice to speak in (F1-F5, M1-M5); empty is the service's
	// default, which is Mary's (F5) - so Gordi sets its own.
	Voice string
	HTTP  *http.Client

	// wavOnly is set once speech is asked for WAV only: the player can't play Opus, or the
	// service refused it.
	wavOnly atomic.Bool
}

// NewClient returns a Client for baseURL. sttModel may be empty for DefaultSTTModel.
func NewClient(baseURL, apiKey, sttModel string) *Client {
	if sttModel == "" {
		sttModel = DefaultSTTModel
	}
	return &Client{BaseURL: baseURL, APIKey: apiKey, STTModel: sttModel, HTTP: &http.Client{Timeout: 2 * time.Minute}}
}

// endpoint resolves path against BaseURL the way the chat client does: a base ending in /v1
// takes the path as is, any other gets /v1 in front.
func (c *Client) endpoint(path string) string {
	base := strings.TrimRight(c.BaseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + path
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("X-Client", "claw-code-go")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s: %d %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// Transcribe returns the text spoken in the WAV file at path.
func (c *Client) Transcribe(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	_ = w.WriteField("model", c.STTModel)
	_ = w.WriteField("response_format", "json")
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/audio/transcriptions"), &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	body, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("transcribe: %w", err)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("transcribe: unexpected answer: %.200s", body)
	}
	return dropMarkers(out.Text), nil
}

var reMarker = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)`)

// dropMarkers removes Whisper's bracketed non-speech markers - [BLANK_AUDIO], (wind blowing) - so
// silence comes back as no text rather than a marker pasted into the input.
func dropMarkers(text string) string {
	if strings.TrimSpace(reMarker.ReplaceAllString(text, "")) == "" {
		return ""
	}
	return strings.TrimSpace(text)
}

// Synthesize returns text spoken as Ogg Opus, a twentieth of the WAV's bytes, so a chunk arrives
// faster than it plays over a slow link; or as a WAV when Opus can't be had. The bytes say which.
func (c *Client) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if !c.wavOnly.Load() {
		audio, err := c.synthesize(ctx, text, "opus")
		var refused *formatRefused
		if !errors.As(err, &refused) {
			return audio, err
		}
		c.wavOnly.Store(true) // a service that only speaks WAV: ask for it from now on
	}
	return c.synthesize(ctx, text, "wav")
}

// WAVOnly makes Synthesize ask for WAV, for a player that can't play Opus.
func (c *Client) WAVOnly() { c.wavOnly.Store(true) }

// formatRefused is a speech service's 400 for a response_format it doesn't make.
type formatRefused struct{ err error }

func (e *formatRefused) Error() string { return e.err.Error() }

func (c *Client) synthesize(ctx context.Context, text, format string) ([]byte, error) {
	body := map[string]string{"model": "supertonic", "input": text, "response_format": format}
	if c.Voice != "" {
		body["voice"] = c.Voice
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/audio/speech"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	audio, err := c.do(req)
	if err != nil {
		err = fmt.Errorf("speak: %w", err)
		if format != "wav" && strings.Contains(err.Error(), ": 400 ") && strings.Contains(err.Error(), "response_format") {
			return nil, &formatRefused{err}
		}
		return nil, err
	}
	return audio, nil
}
