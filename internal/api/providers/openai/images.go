package openai

import "fmt"

// images.go: noticing when the server dropped the images it was sent.
//
// kronk, given images for a model that has no vision projector, keeps a request's text parts and
// drops its image parts, with no error (kronk v1.32.5, model/chat.go prepareTextContext). The model
// then reads "here is the image" with nothing after it, and can describe a picture it never saw.
// kronk marks each streamed chunk with the path it took - object "chat.media" when it used the
// images, "chat.completion.chunk" for text - and its fingerprint, "fp_kronk". Other servers say
// "chat.completion.chunk" whether or not they saw an image, so only kronk's word counts.

const (
	kronkFingerprint = "fp_kronk"
	objectChatMedia  = "chat.media"
)

// imagesDropped reports whether the server took a request that carried sentImages images down
// its text-only path, judged from the first chunk of its reply.
func imagesDropped(chunk oaiChunk, sentImages int) bool {
	return sentImages > 0 && (chunk.Object != objectChatMedia && chunk.SystemFingerprint == kronkFingerprint)
}

// droppedImagesError is what the operator reads when the server dropped a request's images.
func droppedImagesError(n int) string {
	return fmt.Sprintf("the model didn't get the %d image(s) it was sent: the server kept the text and dropped them, "+
		"so its model can't see (no vision projector). From now on each image goes as a note saying so - send your message again", n)
}

// noImageNote stands in for an image once the server has shown it drops them.
const noImageNote = "[An image was here, but this model can't see images, so it wasn't sent]"

// countImageParts is how many image_url parts the messages carry.
func countImageParts(msgs []oaiMessage) int {
	n := 0
	for _, m := range msgs {
		for _, p := range m.Parts {
			if p.Type == "image_url" {
				n++
			}
		}
	}
	return n
}

// withoutImages is msgs with each image_url part replaced by noImageNote, and each message left
// with only text as plain content again.
func withoutImages(msgs []oaiMessage) []oaiMessage {
	out := make([]oaiMessage, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if len(m.Parts) == 0 {
			continue
		}
		var text string
		for _, p := range m.Parts {
			s := p.Text
			if p.Type == "image_url" {
				s = noImageNote
			}
			if s == "" {
				continue
			}
			if text != "" {
				text += "\n"
			}
			text += s
		}
		out[i].Parts = nil
		out[i].Content = strPtr(text)
	}
	return out
}
