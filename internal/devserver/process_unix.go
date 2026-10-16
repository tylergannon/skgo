//go:build !windows

package devserver

import (
	"errors"
	"os/exec"
	"syscall"
)

func ownProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func stopProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	e := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(e, syscall.ESRCH) {
		return nil
	}
	return e
}
