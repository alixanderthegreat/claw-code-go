package voice

import (
	"errors"
	"os/exec"
	"path/filepath"
)

// Check reports what this machine would record and play with, or why voice can't work here: no
// recorder or no player on PATH. It runs nothing.
func Check() (recorder, player string, err error) {
	for _, r := range recorders {
		if _, err := exec.LookPath(r.name); err == nil {
			recorder = r.name
			break
		}
	}
	if recorder == "" {
		return "", "", errors.New("no recorder found: install pw-record (PipeWire) or arecord (alsa-utils)")
	}
	p, err := findPlayer()
	if err != nil {
		return recorder, "", err
	}
	return recorder, filepath.Base(p.bin), nil
}
