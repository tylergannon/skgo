package devserver

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	requestWait  = 30 * time.Second
	requestDrain = time.Second
)

var errRequestUnavailable = errors.New("development application unavailable")

// requestAdmission belongs to the public listener, not either replaceable child.
// Existing requests may still call between Kit and Go while new requests wait.
type requestAdmission struct {
	mu      sync.Mutex
	ready   chan struct{}
	drained chan struct{}
	active  map[uint64]context.CancelFunc
	next    uint64
	closed  bool
}

func (a *requestAdmission) enter(ctx context.Context, timeout time.Duration) (context.Context, func(), error) {
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		a.mu.Lock()
		if err := ctx.Err(); err != nil {
			a.mu.Unlock()
			return nil, nil, err
		}
		if a.closed {
			a.mu.Unlock()
			return nil, nil, errRequestUnavailable
		}
		if a.ready == nil {
			if len(a.active) == 0 {
				a.drained = make(chan struct{})
			}
			if a.active == nil {
				a.active = make(map[uint64]context.CancelFunc)
			}
			a.next++
			id := a.next
			request, cancel := context.WithCancel(ctx)
			a.active[id] = cancel
			a.mu.Unlock()
			return request, func() {
				cancel()
				a.mu.Lock()
				delete(a.active, id)
				if len(a.active) == 0 {
					close(a.drained)
				}
				a.mu.Unlock()
			}, nil
		}
		ready := a.ready
		a.mu.Unlock()
		if timer == nil {
			timer = time.NewTimer(timeout)
		}
		var err error
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-timer.C:
			err = errRequestUnavailable
		case <-ready:
		}
		if err != nil {
			return nil, nil, err
		}
	}
}

// pause stops first dispatch, then gives admitted responses a bounded drain.
// On expiry streams are canceled; possibly executed actions are never replayed.
func (a *requestAdmission) pause(ctx context.Context, grace time.Duration) error {
	a.mu.Lock()
	if a.ready == nil && !a.closed {
		a.ready = make(chan struct{})
	}
	drained := a.drained
	active := len(a.active)
	a.mu.Unlock()
	if active == 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-drained:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		a.mu.Lock()
		for _, cancel := range a.active {
			cancel()
		}
		a.mu.Unlock()
		return ctx.Err()
	}
}

func (a *requestAdmission) resume() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ready != nil {
		close(a.ready)
		a.ready = nil
	}
}

func (a *requestAdmission) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	if a.ready != nil {
		close(a.ready)
		a.ready = nil
	}
	for _, cancel := range a.active {
		cancel()
	}
}
