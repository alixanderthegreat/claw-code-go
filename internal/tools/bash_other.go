//go:build !unix

package tools

import "os/exec"

func detach(*exec.Cmd) {}
