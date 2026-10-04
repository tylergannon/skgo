package dev

import (
	"bytes"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	// Bound Cmd.Wait when descendants keep inherited stdout/stderr pipes open.
	processPipeWaitDelay = 250 * time.Millisecond
	processKillWait      = time.Second
)

// Process is one child this package started and owns by PID. It has its own
// process group, so stopping it stops what it spawned (a package manager
// launcher, Vite's workers) and nothing else on the machine.
type Process struct {
	cmd  *exec.Cmd
	done chan struct{}
	tail *tailBuffer

	mu   sync.Mutex
	err  error
	stop sync.Once
}

func StartProcess(cmd *exec.Cmd, stdout, stderr io.Writer) (*Process, error) {
	tail := &tailBuffer{limit: 8 << 10}
	cmd.Stdout = io.MultiWriter(stdout, tail)
	cmd.Stderr = io.MultiWriter(stderr, tail)
	ownGroup(cmd)
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = processPipeWaitDelay
	}
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

// Stop asks the owned process group to terminate and kills it if it does not.
// Done and ExitError describe only the leader; Stop observes the group through
// its grace period and bounded post-kill settlement.
func (p *Process) Stop(grace time.Duration) {
	p.stop.Do(func() {
		if p.exited() && !processGroupExists(p.cmd) {
			return
		}
		signalGroup(p.cmd, false)
		graceTimer := time.NewTimer(grace)
		defer graceTimer.Stop()
		poll := time.NewTicker(10 * time.Millisecond)
		defer poll.Stop()

		for {
			if p.exited() && !processGroupExists(p.cmd) {
				return
			}

			select {
			case <-poll.C:
			case <-graceTimer.C:
				if !p.exited() || processGroupExists(p.cmd) {
					signalGroup(p.cmd, true)
				}
				<-p.done
				waitForProcessGroupExit(p.cmd, processKillWait)
				return
			}
		}
	})
}

func waitForProcessGroupExit(cmd *exec.Cmd, timeout time.Duration) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for processGroupExists(cmd) {
		select {
		case <-poll.C:
		case <-deadline.C:
			return
		}
	}
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
