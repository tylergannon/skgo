//go:build unix

package dev

import (
	"os/exec"
	"syscall"
)

func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func signalGroup(cmd *exec.Cmd, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	// A negative pid addresses the whole group, which Setpgid made this
	// child's own.
	if syscall.Kill(-cmd.Process.Pid, sig) != nil {
		_ = cmd.Process.Signal(sig)
	}
}
