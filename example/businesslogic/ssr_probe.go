package businesslogic

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SSRProbe lets the browser hold work across an abandoned document request.
// Its gates make cancellation and late completion observable without timing
// guesses. A deadline also releases fixtures if a browser test fails midway.
type SSRProbe struct {
	mu                                                 sync.Mutex
	oldStarted, cancelled, oldFinished, currentStarted bool
	oldRelease, currentRelease                         chan struct{}
	oldOnce, currentOnce                               sync.Once
}

var ssrProbes sync.Map

func RenderProbe(group string) *SSRProbe {
	p := &SSRProbe{oldRelease: make(chan struct{}), currentRelease: make(chan struct{})}
	stored, loaded := ssrProbes.LoadOrStore(group, p)
	if loaded {
		return stored.(*SSRProbe)
	}
	time.AfterFunc(20*time.Second, func() {
		p.Release("abandoned")
		p.Release("current")
		ssrProbes.CompareAndDelete(group, p)
	})
	return p
}

func (p *SSRProbe) Status() map[string]bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]bool{"started": p.oldStarted, "cancelled": p.cancelled, "finished": p.oldFinished, "currentStarted": p.currentStarted}
}

func (p *SSRProbe) Release(which string) {
	if which == "abandoned" {
		p.oldOnce.Do(func() { close(p.oldRelease) })
	}
	if which == "current" {
		p.currentOnce.Do(func() { close(p.currentRelease) })
	}
}

func (p *SSRProbe) Value(ctx context.Context, group, which string) (string, error) {
	switch which {
	case "abandoned":
		p.mu.Lock()
		p.oldStarted = true
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.cancelled = true
			p.mu.Unlock()
		case <-p.oldRelease:
			return "", fmt.Errorf("the abandoned request was never cancelled")
		}
		// Deliberately finish after cancellation: a real driver may return late.
		<-p.oldRelease
		p.mu.Lock()
		p.oldFinished = true
		p.mu.Unlock()
		return "Abandoned: " + group, nil
	case "current":
		p.mu.Lock()
		p.currentStarted = true
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-p.currentRelease:
			return "Current: " + group, nil
		}
	default:
		return "", fmt.Errorf("unknown probe phase %q", which)
	}
}
