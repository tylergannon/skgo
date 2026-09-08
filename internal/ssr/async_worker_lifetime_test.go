package ssr

import (
	"context"
	"testing"
	"time"
)

// TestWorkerHoldsItsHostSlotUntilCompletionIsHandedOff locks down bounded
// worker lifetime for start()'s goroutine: the global host slot it consumes
// must stay held for as long as the goroutine itself is alive, not just for
// as long as the Go I/O (job.call) is running. If the slot were released the
// instant callHost returns -- before the goroutine has actually delivered its
// result to w.completed -- a new job could acquire that freed slot and start
// running while the old worker is still alive, blocked trying to hand off a
// result nobody has read yet. That decouples "goroutines alive" from "slots
// in use", so the slot budget the renderWork doc comment promises ("Limit
// actual Go operations across the engine") no longer bounds the number of
// live goroutines, only the number doing active I/O.
func TestWorkerHoldsItsHostSlotUntilCompletionIsHandedOff(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{} // drain() always acquires the slot before calling start()

	entered := make(chan struct{})
	w := &renderWork{
		ctx:       context.Background(),
		slots:     slots,
		completed: make(chan hostCompletion), // unbuffered: nobody is reading yet
		queue: []*hostJob{{call: func(context.Context) ([]byte, error) {
			close(entered)
			return []byte("x"), nil
		}}},
	}
	w.start()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("job never ran")
	}

	// The handler already returned. Nobody has read w.completed, simulating a
	// render goroutine momentarily busy elsewhere (running JS, launching other
	// jobs). The worker must still be alive and still holding its slot.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(slots) == 0 {
			t.Fatal("slot released before its completion was delivered")
		}
		time.Sleep(2 * time.Millisecond)
	}

	select {
	case c := <-w.completed:
		if string(c.answer) != "x" {
			t.Fatalf("answer = %q", c.answer)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker never attempted delivery")
	}

	deadline = time.Now().Add(2 * time.Second)
	for len(slots) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("slot was never released after delivery")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCancelledWorkerReleasesItsSlotWithoutDeadlock is the negative control
// for the fix: holding the slot through delivery must not turn into a
// deadlock when nobody ever reads w.completed. A cancelled render's worker
// has to abandon delivery via ctx.Done and still release its slot promptly.
func TestCancelledWorkerReleasesItsSlotWithoutDeadlock(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	w := &renderWork{
		ctx:       ctx,
		slots:     slots,
		completed: make(chan hostCompletion), // deliberately never read
		queue: []*hostJob{{call: func(context.Context) ([]byte, error) {
			close(entered)
			return []byte("x"), nil
		}}},
	}
	w.start()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("job never ran")
	}

	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for len(slots) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("cancelled worker never released its slot; delivery abandonment deadlocked")
		}
		time.Sleep(time.Millisecond)
	}
}
