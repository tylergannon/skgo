//go:build !darwin && !linux

package pluginbuild

import (
	"context"
	"os/exec"
)

func ChildCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
