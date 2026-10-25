package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestMessageAttachesTheImageInIt: a message with an image's path in it carries the image, and
// the token estimate counts it.
func TestMessageAttachesTheImageInIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.png")
	os.WriteFile(path, []byte("\x89PNG fake"), 0o644)
	client := &scriptedClient{}
	loop := newScriptedLoop(client, nil)
	loop.appendUserMessage("what's in " + path + "?")
	msg := loop.Session.Messages[len(loop.Session.Messages)-1]
	if len(msg.Content) != 2 || msg.Content[1].Type != "image" || msg.Content[1].Source.MediaType != "image/png" {
		t.Fatalf("the message: %+v", msg.Content)
	}
	if n := countImages(loop.Session.Messages); n != 1 {
		t.Errorf("countImages = %d", n)
	}
	if est := EstimateTokens(loop.Session.Messages); est < 1500 {
		t.Errorf("EstimateTokens = %d, want the image counted", est)
	}
}

// TestReadFileOnAnImageReturnsTheImage: read_file on an image gives the model the image, not its
// bytes as text, and counts as reading it for a later write.
func TestReadFileOnAnImageReturnsTheImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logo.png")
	os.WriteFile(path, []byte("\x89PNG fake"), 0o644)
	client := &scriptedClient{tool: "read_file", input: map[string]any{"path": path}}
	loop := newScriptedLoop(client, nil)
	events := make(chan TurnEvent, 64)
	go func() {
		loop.SendMessageStreaming(context.Background(), "look at the logo", events)
		close(events)
	}()
	for range events {
	}
	var found bool
	for _, m := range loop.Session.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_result" && len(b.Content) == 2 && b.Content[1].Type == "image" && !b.IsError {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no image in a tool result: %+v", loop.Session.Messages)
	}
	if !loop.ReadFiles[path] && loop.requireReadBeforeWrite(path) != nil {
		t.Error("reading the image didn't count as reading it")
	}
}
