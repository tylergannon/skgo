// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in GOPLS_LICENSE.
//
// Semantic port of gopls memoize.Promise (x/tools v0.52.0, 16517d9): equivalent
// callers share computation; individual cancellation detaches one waiter, the
// last waiter cancels work, and abandoned work may be retried by a later caller.
package gen

import (
	"context"
	"sync"
)

type generationFuture struct {
	mu       sync.Mutex
	run      *generationRun
	result   generationPlan
	err      error
	complete bool
}
type generationRun struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
}

func newGenerationFuture() *generationFuture { return &generationFuture{} }
func (f *generationFuture) finished() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.complete || f.run == nil
}
func (f *generationFuture) get(ctx context.Context, compute func(context.Context) (generationPlan, error)) (generationPlan, error) {
	if err := ctx.Err(); err != nil {
		return generationPlan{}, err
	}
	f.mu.Lock()
	if f.complete {
		v, e := f.result, f.err
		f.mu.Unlock()
		return v, e
	}
	r := f.run
	if r == nil {
		work, cancel := context.WithCancel(context.WithoutCancel(ctx))
		r = &generationRun{ctx: work, cancel: cancel, done: make(chan struct{})}
		f.run = r
		go func() {
			value, err := compute(work)
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.run == r {
				if work.Err() == nil {
					f.result = value
					f.err = err
					f.complete = true
				}
				f.run = nil
			}
			close(r.done)
		}()
	}
	r.waiters++
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		f.mu.Lock()
		r.waiters--
		if r.waiters == 0 && f.run == r {
			r.cancel()
			f.run = nil
		}
		f.mu.Unlock()
		return generationPlan{}, ctx.Err()
	case <-r.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.complete {
			return f.result, f.err
		}
		return generationPlan{}, r.ctx.Err()
	}
}
