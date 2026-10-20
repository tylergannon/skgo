//go:build !windows

package devserver

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// claimInstance holds a lock on work for as long as this process lives. The
// lock is taken before the file gets the name sweepInstances looks for.
func claimInstance(work string) (*os.File, error) {
	f, e := os.CreateTemp(work, "claim-")
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e == nil {
		e = os.Rename(f.Name(), filepath.Join(work, instanceLock))
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}

// sweepInstances removes the work directories of sessions that were killed
// before they could remove their own: nothing holds their lock any more.
func sweepInstances(parent string) {
	locks, _ := filepath.Glob(filepath.Join(parent, "skgo-*", instanceLock))
	for _, lock := range locks {
		f, e := os.Open(lock)
		if e != nil {
			continue
		}
		if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			_ = os.RemoveAll(filepath.Dir(lock))
		}
		f.Close()
	}
}
