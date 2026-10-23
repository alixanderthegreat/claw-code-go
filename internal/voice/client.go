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
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	HTTP     *http.Client
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

// Synthesize returns text spoken as a WAV.
func (c *Client) Synthesize(ctx context.Context, text string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]string{"model": "supertonic", "input": text, "response_format": "wav"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/audio/speech"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("speak: %w", err)
	}
	return body, nil
}
