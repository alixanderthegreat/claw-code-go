//go:build unix

package tools

import (
	"os/exec"
	"syscall"
)

// detach runs cmd in a session of its own - no controlling terminal, so nothing it starts can open
// /dev/tty and prompt over the TUI - and makes cancelling it kill its whole process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
