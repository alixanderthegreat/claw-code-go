package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig puts body at the global config path under a temp HOME, at mode.
func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path, err := GlobalConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // past the umask
		t.Fatal(err)
	}
	return path
}

func TestEnsureGlobalWritesA0600Template(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	created, path, err := EnsureGlobal()
	if err != nil || !created {
		t.Fatalf("created=%v path=%q err=%v, want a fresh template", created, path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("template is mode %04o, want 0600 (it holds a key)", info.Mode().Perm())
	}
	gc, err := LoadGlobal()
	if err != nil {
		t.Fatalf("the template doesn't parse: %v", err)
	}
	if gc.Model != "" {
		t.Errorf("template model is %q, want empty (not a frontier-model guess)", gc.Model)
	}
	if gc.ContextWindow != templateContextWindow {
		t.Errorf("template context_window is %d, want %d", gc.ContextWindow, templateContextWindow)
	}

	// A second run must not clobber it or claim to have created it.
	created, _, err = EnsureGlobal()
	if err != nil || created {
		t.Errorf("second EnsureGlobal: created=%v err=%v, want created=false", created, err)
	}
}

func TestValidate(t *testing.T) {
	t.Run("missing file is not an error", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		if err := Validate(); err != nil {
			t.Errorf("no config should be fine, got %v", err)
		}
	})
	t.Run("good config passes", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		writeConfig(t, `{"base_url":"http://x","api_key":"k"}`, 0o600)
		if err := Validate(); err != nil {
			t.Errorf("valid config rejected: %v", err)
		}
	})
	t.Run("broken config is named, not swallowed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		path := writeConfig(t, `{"base_url": "http://x",,}`, 0o600)
		err := Validate()
		if err == nil {
			t.Fatal("a config that doesn't parse should be an error")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("the error should name the file %q, got %v", path, err)
		}
	})
}

func TestWorldReadable(t *testing.T) {
	for _, c := range []struct {
		name    string
		body    string
		mode    os.FileMode
		exposed bool
	}{
		{"key and group-readable", `{"api_key":"k"}`, 0o640, true},
		{"key and world-readable", `{"api_key":"k"}`, 0o644, true},
		{"key but tight", `{"api_key":"k"}`, 0o600, false},
		{"loose but no key", `{"base_url":"http://x"}`, 0o644, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			writeConfig(t, c.body, c.mode)
			exposed, _, err := WorldReadable()
			if err != nil {
				t.Fatal(err)
			}
			if exposed != c.exposed {
				t.Errorf("exposed=%v, want %v (mode %04o)", exposed, c.exposed, c.mode)
			}
		})
	}
}
