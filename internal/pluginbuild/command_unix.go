//go:build darwin || linux

package pluginbuild

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ChildCommand cancels the owned process group, including generator and compiler
// descendants, and bounds waits on inherited output pipes.
func ChildCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		err := syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	c.WaitDelay = time.Second
	return c
}
