package tools

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"claw-code-go/internal/api"
)

// images.go: images as content blocks, for a model that can see them (Jumbo, through its projector).
// They reach the model two ways: a path to one in your message - dragging a file into the terminal
// pastes its path - is attached to the message, and read_file on one returns the image instead of
// its bytes as text.

// MaxImageBytes bounds one image. A screenshot is well under it; the model's projector costs time
// and tokens by resolution, not bytes, but a file this big is rarely meant to be looked at.
const MaxImageBytes = 8 << 20

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// ImageMediaType is path's image type by its extension, or "" when it isn't one.
func ImageMediaType(path string) string {
	return imageTypes[strings.ToLower(filepath.Ext(path))]
}

// LoadImage reads the image at path as a content block, with its size in bytes.
func LoadImage(path string) (api.ContentBlock, int, error) {
	mediaType := ImageMediaType(path)
	if mediaType == "" {
		return api.ContentBlock{}, 0, fmt.Errorf("%s is not an image this model reads (png, jpeg, gif, webp)", path)
	}
	if err := requireWithinDispatchBoundary(path); err != nil {
		return api.ContentBlock{}, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return api.ContentBlock{}, 0, err
	}
	if info.Size() > MaxImageBytes {
		return api.ContentBlock{}, 0, fmt.Errorf("%s is %d MB, over the %d MB an image may be", path, info.Size()>>20, MaxImageBytes>>20)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return api.ContentBlock{}, 0, err
	}
	return api.ContentBlock{
		Type:   "image",
		Source: &api.ImageSource{Type: "base64", MediaType: mediaType, Data: base64.StdEncoding.EncodeToString(raw)},
	}, len(raw), nil
}

// ImagePathsIn finds paths to image files that exist in text, in order, each once. A path may be
// bare, quoted ('...' or "..."), have its spaces escaped with backslashes, or be a file:// URI -
// the forms terminals paste for a dropped file.
func ImagePathsIn(text string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, word := range shellWords(text) {
		word = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(word), "file://"))
		if u, err := url.PathUnescape(word); err == nil {
			word = u
		}
		word = strings.TrimRight(word, ".,;:!?)")
		if strings.HasPrefix(word, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				word = filepath.Join(home, word[2:])
			}
		}
		if ImageMediaType(word) == "" || seen[word] {
			continue
		}
		if info, err := os.Stat(word); err == nil && info.Mode().IsRegular() {
			seen[word] = true
			paths = append(paths, word)
		}
	}
	return paths
}

// shellWords splits text on whitespace the way a shell would for the forms a pasted path takes:
// quotes group, a backslash escapes the next character. A quote only opens at the start of a word,
// so the apostrophe in "what's" is just a letter.
func shellWords(text string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	escaped, inWord := false, false
	for _, r := range text {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case (r == '\'' || r == '"') && !inWord:
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}
