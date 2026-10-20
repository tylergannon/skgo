//go:build windows

package devserver

import (
	"os"
	"os/exec"
)

func ownProcess(cmd *exec.Cmd) {}
func stopProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func claimInstance(work string) (*os.File, error) { return nil, nil }
func sweepInstances(parent string)                {}
