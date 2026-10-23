package voice

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Recorder commands, in the order tried: PipeWire's own, then ALSA's. Both write a 16 kHz mono
// WAV - what Whisper wants - and finish the file when interrupted.
var recorders = []struct {
	name string
	args func(path string) []string
}{
	{"pw-record", func(p string) []string { return []string{"--rate", "16000", "--channels", "1", "--format", "s16", p} }},
	{"arecord", func(p string) []string {
		return []string{"-q", "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "wav", p}
	}},
}

// Recording is the mic being recorded to a temporary WAV.
type Recording struct {
	cmd  *exec.Cmd
	path string
	done chan error
}

// StartRecording starts recording the default mic.
func StartRecording() (*Recording, error) {
	for _, r := range recorders {
		bin, err := exec.LookPath(r.name)
		if err != nil {
			continue
		}
		f, err := os.CreateTemp("", "claw-voice-*.wav")
		if err != nil {
			return nil, err
		}
		path := f.Name()
		f.Close()
		cmd := exec.Command(bin, r.args(path)...)
		if err := cmd.Start(); err != nil {
			os.Remove(path)
			return nil, fmt.Errorf("start %s: %w", r.name, err)
		}
		rec := &Recording{cmd: cmd, path: path, done: make(chan error, 1)}
		go func() { rec.done <- cmd.Wait() }()
		return rec, nil
	}
	return nil, errors.New("no recorder found: install pw-record (PipeWire) or arecord (alsa-utils)")
}

// Stop ends the recording and returns the WAV's path; the caller removes it. A recorder that
// already died is an error, and so is one that ignores the interrupt for two seconds.
func (r *Recording) Stop() (string, error) {
	select {
	case err := <-r.done:
		os.Remove(r.path)
		return "", fmt.Errorf("recorder exited early: %v", err)
	default:
	}
	_ = r.cmd.Process.Signal(syscall.SIGINT)
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		_ = r.cmd.Process.Kill()
		<-r.done
	}
	if st, err := os.Stat(r.path); err != nil || st.Size() <= 44 { // a WAV header and no audio
		os.Remove(r.path)
		return "", errors.New("nothing was recorded - check the mic is the default input")
	}
	return r.path, nil
}

// Cancel ends the recording and throws it away.
func (r *Recording) Cancel() {
	_ = r.cmd.Process.Kill()
	<-r.done
	os.Remove(r.path)
}
