package ssr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

// singleCallBundle makes exactly one host call per render, so one render
// leaks exactly one slot of the engine's shared host-call budget.
const singleCallBundle = `
globalThis.__skgo_ping = () => 'ok';
globalThis.__skgo_render = (json) => {
  const result = {done:false};
  __skgo_remote('x', '').then(function(raw) {
    result.body = raw;
    result.done = true;
  }, function(e) { result.failure = String(e); result.done = true; });
  return result;
};`

// TestLeakedHostCallsExhaustTheEnginesSharedBudget establishes a limit rather
// than a bug: maxHostCalls (32) bounds Go operations across the *whole*
// engine, not per render, and it deliberately counts a call whose handler is
// still running after its render was cancelled (the package doc on
// maxHostCalls and TestCancelledRenderCannotResumeInReusedPool both say so
// explicitly). This test shows the consequence for a caller: enough
// noncooperative, cancelled handlers accumulate and starve a later,
// completely unrelated render's own host call, even though the pool of
// *runtimes* (Engine.Size) has long since recovered. The caller's own context
// deadline is what bounds the wait — there is no engine-side fairness or
// eviction of stale slots.
func TestLeakedHostCallsExhaustTheEnginesSharedBudget(t *testing.T) {
	engine, err := ssr.New("leak.js", []byte(singleCallBundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	deadline := time.After(10 * time.Second)

	// Pool size 1, so only one render is ever in flight. Each is cancelled
	// the instant its handler has entered (and therefore has already taken
	// its slot from the engine-wide budget) but the handler itself never
	// looks at ctx and blocks on release, so the slot is never returned even
	// though the runtime that started it is discarded right after.
	for i := 0; i < 32; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		entered := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			engine.Render(ctx, "/leak", request(t, "/leak"), ssr.Hosts{Remote: func(context.Context, string, string) ([]byte, error) {
				close(entered)
				<-release
				return answer("STALE"), nil
			}})
		}()
		select {
		case <-entered:
		case <-deadline:
			t.Fatalf("leaked handler %d never entered", i)
		}
		cancel()
		select {
		case <-done:
		case <-deadline:
			t.Fatalf("cancelled render %d never returned", i)
		}
	}

	if created := engine.Created(); created != 32 {
		t.Fatalf("engine created %d runtimes, want 32 discarded-and-replaced runtimes", created)
	}

	// Every one of the engine's 32 host slots is now held by a handler that
	// will never return on its own. A brand-new, otherwise-healthy render
	// needing just one host call must time out on its own deadline rather
	// than proceed — the shared budget gives it nothing to run on.
	starved, starvedCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer starvedCancel()
	_, _, err = engine.Render(starved, "/starved", request(t, "/starved"), ssr.Hosts{Remote: func(_ context.Context, id, _ string) ([]byte, error) {
		return answer(id), nil
	}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the starved render to time out waiting for a host slot, got %v", err)
	}

	// Once the leaked handlers finally return — a slow downstream call
	// completing late, say — the budget recovers on its own and a fresh
	// render proceeds normally.
	close(release)
	recovered, recoveredCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer recoveredCancel()
	result, _, err := engine.Render(recovered, "/recovered", request(t, "/recovered"), ssr.Hosts{Remote: func(_ context.Context, id, _ string) ([]byte, error) {
		return answer(id), nil
	}})
	if err != nil || result.Body == "" {
		t.Fatalf("%+v: %v", result, err)
	}
}
