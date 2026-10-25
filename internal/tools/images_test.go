package tools

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestImagePathsIn: a dropped image's path is found in each form a terminal pastes it, and
// nothing else is: not a missing file, not a text file, not a sentence with an apostrophe.
func TestImagePathsIn(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "shot.png")
	spaced := filepath.Join(dir, "my shot.JPG")
	notes := filepath.Join(dir, "notes.txt")
	for _, p := range []string{plain, spaced, notes} {
		os.WriteFile(p, []byte("x"), 0o644)
	}
	for text, want := range map[string][]string{
		"what's in " + plain + "?":                       {plain},
		"look at '" + spaced + "' please":                 {spaced},
		`look at "` + spaced + `"`:                        {spaced},
		"look at " + strings.ReplaceAll(spaced, " ", `\ `): {spaced},
		"file://" + strings.ReplaceAll(spaced, " ", "%20"): {spaced},
		plain + " and " + plain + " again":               {plain},
		"compare " + plain + ", " + "'" + spaced + "'":    {plain, spaced},
		notes + " and " + filepath.Join(dir, "gone.png"):  nil,
		"it's what we've got":                             nil,
	} {
		if got := ImagePathsIn(text); !reflect.DeepEqual(got, want) {
			t.Errorf("ImagePathsIn(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestLoadImage(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "a.png")
	os.WriteFile(png, []byte("\x89PNG fake"), 0o644)
	block, size, err := LoadImage(png)
	if err != nil || block.Type != "image" || block.Source.MediaType != "image/png" || size != 9 || block.Source.Data == "" {
		t.Fatalf("LoadImage: %+v %d %v", block.Source, size, err)
	}
	big := filepath.Join(dir, "big.png")
	os.WriteFile(big, make([]byte, MaxImageBytes+1), 0o644)
	if _, _, err := LoadImage(big); err == nil {
		t.Error("an image over the limit was loaded")
	}
	if _, _, err := LoadImage(filepath.Join(dir, "a.txt")); err == nil {
		t.Error("a non-image was loaded")
	}
}
