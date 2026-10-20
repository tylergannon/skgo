package devserver

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// instanceLock names the file a live session holds locked in its work directory.
const instanceLock = "session.lock"

type child struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	once sync.Once
	// lifeline is the only writer of the child's stdin. The kernel closes it
	// however this process exits, so a child that waits for end of input
	// cannot outlive the session, even when the session is killed.
	lifeline *os.File
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
	input, lifeline, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	cmd.Stdin = input
	e = cmd.Start()
	input.Close()
	if e != nil {
		lifeline.Close()
		return nil, e
	}
	c := &child{cmd: cmd, done: make(chan struct{}), lifeline: lifeline}
	go func() { c.err = cmd.Wait(); close(c.done) }()
	return c, nil
}
func (c *child) close() {
	if c != nil {
		c.once.Do(func() { _ = stopProcess(c.cmd); <-c.done; c.lifeline.Close() })
	}
}
