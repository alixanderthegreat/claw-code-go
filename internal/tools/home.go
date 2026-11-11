package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// expandHome turns a leading "~" or "~/" into the home directory, as a shell would. The file tools
// get paths the model writes the way it would at a prompt; taken as written, "~/x" was a folder
// named "~" in the working directory, and write_file reported success at a path that wasn't there
// (greatproject #439). "~user" is left alone.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
