package ssr

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
)

// Limit actual Go operations across the engine, including operations from
// cancelled requests whose handlers have not returned yet. Waiting jobs stay
// on the render's queue rather than consuming goroutines.
const maxHostCalls = 32

type hostJob struct {
	call            func(context.Context) ([]byte, error)
	resolve, reject func(any) error
}

type hostCompletion struct {
	job    *hostJob
	answer []byte
	err    error
}

// Only the render goroutine touches this state or any goja value. Workers
// return plain Go data; they never resolve promises themselves.
type renderWork struct {
	ctx         context.Context
	slots       chan struct{}
	queue       []*hostJob
	completed   chan hostCompletion
	outstanding int
}

func (rt *runtime) promise(call func(context.Context) ([]byte, error)) *goja.Promise {
	p, resolve, reject := rt.vm.NewPromise()
	if rt.work == nil {
		_ = reject(rt.vm.NewGoError(fmt.Errorf("skgo: host call outside a render")))
		return p
	}
	rt.work.queue = append(rt.work.queue, &hostJob{call: call, resolve: resolve, reject: reject})
	rt.work.outstanding++
	return p
}

// start consumes a slot already acquired by the render goroutine. The slot is
// held for the goroutine's whole lifetime, including handing the result off,
// so "holds a slot" and "goroutine is alive" stay the same thing: releasing
// it before delivery would let a new job start while this one is still alive
// waiting to deliver, decoupling live goroutines from the slot budget above.
func (w *renderWork) start() {
	job := w.queue[0]
	w.queue[0] = nil
	w.queue = w.queue[1:]
	go func() {
		answer, err := callHost(w.ctx, job.call)
		select {
		case w.completed <- hostCompletion{job: job, answer: answer, err: err}:
		case <-w.ctx.Done():
		}
		<-w.slots
	}()
}

func callHost(ctx context.Context, call func(context.Context) ([]byte, error)) (answer []byte, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("skgo: render host panicked: %v", p)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return call(ctx)
}

func (rt *runtime) complete(c hostCompletion) error {
	rt.work.outstanding--
	if c.err != nil {
		rt.failed = errors.Join(rt.failed, c.err)
		return c.job.reject(rt.vm.NewGoError(c.err))
	}
	return c.job.resolve(string(c.answer))
}
