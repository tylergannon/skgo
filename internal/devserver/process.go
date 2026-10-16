package devserver

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"time"
)

type child struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	once sync.Once
}

func startChild(ctx context.Context, dir, name string, args, env []string, output io.Writer) (*child, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = output
	cmd.Stderr = output
	ownProcess(cmd)
	cmd.Cancel = func() error { return stopProcess(cmd) }
	cmd.WaitDelay = 2 * time.Second
	if e := cmd.Start(); e != nil {
		return nil, e
	}
	c := &child{cmd: cmd, done: make(chan struct{})}
	go func() { c.err = cmd.Wait(); close(c.done) }()
	return c, nil
}
func (c *child) close() {
	if c != nil {
		c.once.Do(func() { _ = stopProcess(c.cmd); <-c.done })
	}
}
