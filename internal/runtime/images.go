package runtime

import (
	"fmt"
	"path/filepath"
	"strings"

	"claw-code-go/internal/api"
	"claw-code-go/internal/tools"
)

// images.go: giving the model images (internal/tools/images.go). An image costs Jumbo ~1,400 tokens
// and, the first time, ~13-19 s of the projector encoding it on compute's CPU; kronk's cache keeps
// it after that, so follow-up turns about the same image are fast.

// attachImages loads the images whose paths are in text, as blocks for the message. One that
// can't be loaded is left out; its path is still in the text, so the model can say so.
func (loop *ConversationLoop) attachImages(text string) []api.ContentBlock {
	var blocks []api.ContentBlock
	for _, path := range tools.ImagePathsIn(text) {
		block, size, err := tools.LoadImage(path)
		if err != nil {
			continue
		}
		blocks = append(blocks, block)
		loop.emitImage("message", path, size)
	}
	return blocks
}

// takeShellImages loads the images ! commands printed since the last message, and clears them.
func (loop *ConversationLoop) takeShellImages() []api.ContentBlock {
	var blocks []api.ContentBlock
	for _, path := range loop.shellImages {
		if block, size, err := tools.LoadImage(path); err == nil {
			blocks = append(blocks, block)
			loop.emitImage("shell", path, size)
		}
	}
	loop.shellImages = nil
	return blocks
}

// readImage is read_file's result for an image: a line saying what it is, and the image.
func (loop *ConversationLoop) readImage(path string) api.ContentBlock {
	block, size, err := tools.LoadImage(path)
	if err != nil {
		return api.ContentBlock{
			Type:    "tool_result",
			Content: []api.ContentBlock{{Type: "text", Text: "Error: read_file: " + err.Error()}},
			IsError: true,
		}
	}
	loop.markRead(path)
	loop.emitImage("read_file", path, size)
	kind := strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
	return api.ContentBlock{
		Type: "tool_result",
		Content: []api.ContentBlock{
			{Type: "text", Text: fmt.Sprintf("%s is an image (%s, %d KB); here it is.", path, kind, (size+1023)/1024)},
			block,
		},
	}
}

func (loop *ConversationLoop) emitImage(source, path string, size int) {
	loop.Telemetry.Send("gordi_image", map[string]any{
		"session": loop.Session.ID,
		"source":  source,
		"type":    strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		"bytes":   size,
	})
}

// countImages is how many images a request carries, wherever they are in it.
func countImages(messages []api.Message) int {
	n := 0
	for _, m := range messages {
		for _, b := range m.Content {
			if b.Type == "image" {
				n++
			}
			for _, inner := range b.Content {
				if inner.Type == "image" {
					n++
				}
			}
		}
	}
	return n
}
