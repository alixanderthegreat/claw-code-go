package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"claw-code-go/internal/api"
)

// imageRequest is a user message carrying one image.
var imageRequest = api.CreateMessageRequest{Messages: []api.Message{{Role: "user", Content: []api.ContentBlock{
	{Type: "text", Text: "what's this?"},
	{Type: "image", Source: &api.ImageSource{Type: "base64", MediaType: "image/png", Data: "QUJD"}},
}}}}

// textRequest is a user message with no image.
var textRequest = api.CreateMessageRequest{Messages: []api.Message{{Role: "user", Content: []api.ContentBlock{
	{Type: "text", Text: "hello"},
}}}}

// fakeServer answers every request with one chunk marked object and fingerprint, and keeps each
// request's body.
func fakeServer(t *testing.T, object, fingerprint string, bodies *[]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, string(b))
		chunk, _ := json.Marshal(map[string]any{"object": object, "system_fingerprint": fingerprint,
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "a red square"}}}})
		w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Model: "m", HTTPClient: srv.Client()}
}

// run streams one request and returns its error message, if any, and its text.
func run(t *testing.T, c *Client, req api.CreateMessageRequest) (errMsg, text string) {
	t.Helper()
	ch, err := c.StreamResponse(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for ev := range ch {
		switch ev.Type {
		case api.EventError:
			errMsg = ev.ErrorMessage
		case api.EventContentBlockDelta:
			text += ev.Delta.Text
		}
	}
	return errMsg, text
}

func TestImagesDropped(t *testing.T) {
	for _, tc := range []struct {
		name                string
		object, fingerprint string
		sent                int
		want                bool
	}{
		{"kronk used the image", "chat.media", "fp_kronk", 1, false},
		{"kronk dropped the image", "chat.completion.chunk", "fp_kronk", 1, true},
		{"kronk, no image sent", "chat.completion.chunk", "fp_kronk", 0, false},
		{"another server, image sent", "chat.completion.chunk", "fp_44709d6fcb", 1, false},
		{"a server with no fingerprint", "chat.completion.chunk", "", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunk := oaiChunk{Object: tc.object, SystemFingerprint: tc.fingerprint}
			if got := imagesDropped(chunk, tc.sent); got != tc.want {
				t.Errorf("imagesDropped(%q, %q, %d) = %v, want %v", tc.object, tc.fingerprint, tc.sent, got, tc.want)
			}
		})
	}
}

// TestDroppedImagesStopTheTurnThenGoAsNotes: kronk dropping an image stops the turn before the
// model's answer is shown, and from then on the image goes as a note, so the conversation carries on.
func TestDroppedImagesStopTheTurnThenGoAsNotes(t *testing.T) {
	var bodies []string
	c := fakeServer(t, "chat.completion.chunk", "fp_kronk", &bodies)

	errMsg, text := run(t, c, imageRequest)
	if !strings.Contains(errMsg, "didn't get the 1 image") {
		t.Fatalf("error = %q, want one saying the image was dropped", errMsg)
	}
	if text != "" {
		t.Errorf("text = %q: the answer to an image the model never saw was shown", text)
	}

	errMsg, text = run(t, c, imageRequest)
	if errMsg != "" || text != "a red square" {
		t.Fatalf("second request: error %q, text %q; want no error and the answer", errMsg, text)
	}
	if strings.Contains(bodies[1], "image_url") || !strings.Contains(bodies[1], noImageNote) {
		t.Errorf("second request = %s, want the image replaced by the note", bodies[1])
	}
}

// TestSeeingAndTextOnlyArePassedThrough: a kronk that used the image, a text-only request and
// another server's reply all go through untouched, with the image still sent.
func TestSeeingAndTextOnlyArePassedThrough(t *testing.T) {
	for _, tc := range []struct {
		name                string
		object, fingerprint string
		req                 api.CreateMessageRequest
	}{
		{"kronk with vision", "chat.media", "fp_kronk", imageRequest},
		{"kronk, text only", "chat.completion.chunk", "fp_kronk", textRequest},
		{"another server", "chat.completion.chunk", "fp_44709d6fcb", imageRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies []string
			c := fakeServer(t, tc.object, tc.fingerprint, &bodies)
			for i := range 2 {
				if errMsg, text := run(t, c, tc.req); errMsg != "" || text != "a red square" {
					t.Fatalf("request %d: error %q, text %q; want no error and the answer", i+1, errMsg, text)
				}
			}
			if tc.req.Messages[0].Content[len(tc.req.Messages[0].Content)-1].Type == "image" &&
				!strings.Contains(bodies[1], "image_url") {
				t.Errorf("second request = %s, want the image still sent", bodies[1])
			}
		})
	}
}

// TestWithoutImagesKeepsTheText: a message's text survives, each image becomes the note, and a
// tool's image message reads as text.
func TestWithoutImagesKeepsTheText(t *testing.T) {
	msgs := withoutImages(convertMessages("", []api.Message{
		{Role: "user", Content: []api.ContentBlock{
			{Type: "text", Text: "compare these"},
			{Type: "image", Source: &api.ImageSource{MediaType: "image/png", Data: "QQ"}},
			{Type: "image", Source: &api.ImageSource{MediaType: "image/png", Data: "Qg"}},
		}},
	}))
	if countImageParts(msgs) != 0 {
		t.Fatalf("images left: %+v", msgs)
	}
	want := "compare these\n" + noImageNote + "\n" + noImageNote
	if len(msgs) != 1 || msgs[0].Content == nil || *msgs[0].Content != want {
		t.Fatalf("messages = %+v, want one with content %q", msgs, want)
	}
}
