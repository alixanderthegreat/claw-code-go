package telemetry

import "runtime/debug"

// Build is what this Gordi was built from, for its gordi_start event: the commit `go install`
// stamped into the binary (revision, empty for `go run` or a build outside git), whether that
// checkout had uncommitted changes (modified), and the Go version. Grafana compares revision with
// main to show how far the installed Gordi is behind.
func Build() map[string]any {
	f := map[string]any{"revision": "", "modified": false, "go": ""}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return f
	}
	f["go"] = info.GoVersion
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			f["revision"] = s.Value
		case "vcs.modified":
			f["modified"] = s.Value == "true"
		}
	}
	return f
}
