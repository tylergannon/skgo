package dev

import (
	"bytes"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Process is one child this package started and owns by PID. It has its own
// process group, so stopping it stops what it spawned (a package manager
// launcher, Vite's workers) and nothing else on the machine.
type Process struct {
	cmd  *exec.Cmd
	done chan struct{}
	tail *tailBuffer

	mu  sync.Mutex
	err error
}

func StartProcess(cmd *exec.Cmd, stdout, stderr io.Writer) (*Process, error) {
	tail := &tailBuffer{limit: 8 << 10}
	cmd.Stdout = io.MultiWriter(stdout, tail)
	cmd.Stderr = io.MultiWriter(stderr, tail)
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &Process{cmd: cmd, done: make(chan struct{}), tail: tail}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.done)
	}()
	return p, nil
}

// PID is the exact process id this package started.
func (p *Process) PID() int { return p.cmd.Process.Pid }

// Done is closed when the process has exited.
func (p *Process) Done() <-chan struct{} { return p.done }

func (p *Process) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// ExitError is what Wait reported, once Done is closed.
func (p *Process) ExitError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Stop asks the process group to terminate and kills it if it does not.
func (p *Process) Stop(grace time.Duration) {
	if p.exited() {
		return
	}
	signalGroup(p.cmd, false)
	select {
	case <-p.done:
		return
	case <-time.After(grace):
	}
	signalGroup(p.cmd, true)
	<-p.done
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   bytes.Buffer
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(b)
	if extra := t.buf.Len() - t.limit; extra > 0 {
		t.buf.Next(extra)
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}
