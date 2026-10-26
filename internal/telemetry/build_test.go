package telemetry

import (
	"strings"
	"testing"
)

// A test binary carries no vcs stamp, but always its Go version, and every field is there.
func TestBuild(t *testing.T) {
	f := Build()
	if !strings.HasPrefix(f["go"].(string), "go") {
		t.Fatalf("go = %v", f["go"])
	}
	if _, ok := f["revision"].(string); !ok {
		t.Fatalf("revision = %v", f["revision"])
	}
	if _, ok := f["modified"].(bool); !ok {
		t.Fatalf("modified = %v", f["modified"])
	}
}
