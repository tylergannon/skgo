package ssr_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

const parallelBundle = `
var history = [];
globalThis.__skgo_ping = () => 'ok';
globalThis.__skgo_render = (json) => {
 const req = JSON.parse(json);
 const result = {done:false};
 Promise.all(['amber', 'birch', 'cobalt'].map(key =>
   __skgo_remote(key, req.route_id).then(raw => JSON.parse(raw).v)
 )).then(values => {
   history.push(req.route_id);
   result.body = values.join(',') + ';' + history.join(',');
   result.done = true;
 }, error => { result.failure = String(error); result.done = true; });
 return result;
};`

func TestIndependentHostCallsStartBeforeAnyCompletes(t *testing.T) {
	engine, err := ssr.New("parallel.js", []byte(parallelBundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var started atomic.Int32
	gate := make(chan struct{})
	result, _, err := engine.Render(ctx, "/first", request(t, "/first"), ssr.Hosts{Remote: func(ctx context.Context, id, payload string) ([]byte, error) {
		if started.Add(1) == 3 {
			close(gate)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-gate:
		}
		return answer(id), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Body != "amber,birch,cobalt;/first" {
		t.Fatal(result.Body)
	}
}

func TestCancelledRenderCannotResumeInReusedPool(t *testing.T) {
	engine, err := ssr.New("parallel.js", []byte(parallelBundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	late := make(chan struct{}, 3)
	done := make(chan error, 1)
	go func() {
		_, _, err := engine.Render(ctx, "/abandoned", request(t, "/abandoned"), ssr.Hosts{Remote: func(context.Context, string, string) ([]byte, error) {
			entered <- struct{}{}
			<-release // deliberately ignores cancellation, like a noncooperative driver
			late <- struct{}{}
			return answer("STALE"), nil
		}})
		done <- err
	}()
	deadline := time.After(3 * time.Second)
	for range 3 {
		select {
		case <-entered:
		case <-deadline:
			t.Fatal("calls never overlapped")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-deadline:
		t.Fatal("render did not cancel")
	}
	// The sole pool slot must be available even though all old handlers are still blocked.
	fresh := ssr.Hosts{Remote: func(_ context.Context, id, _ string) ([]byte, error) { return answer(id), nil }}
	result, _, err := engine.Render(context.Background(), "/fresh", request(t, "/fresh"), fresh)
	if err != nil || result.Body != "amber,birch,cobalt;/fresh" {
		t.Fatalf("%+v: %v", result, err)
	}
	close(release)
	for range 3 {
		<-late
	}
	result, _, err = engine.Render(context.Background(), "/after", request(t, "/after"), fresh)
	if err != nil || result.Body != "amber,birch,cobalt;/fresh,/after" {
		t.Fatalf("%+v: %v", result, err)
	}
	if engine.Created() != 2 {
		t.Fatalf("created %d, want one discarded runtime and one reused replacement", engine.Created())
	}
}

func TestWaitingForAPoolSlotCanBeCancelled(t *testing.T) {
	engine, err := ssr.New("parallel.js", []byte(parallelBundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = engine.Render(ctx, "/held", request(t, "/held"), ssr.Hosts{Remote: func(ctx context.Context, _, _ string) ([]byte, error) {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}})
	}()
	<-entered
	waiting, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	_, _, err = engine.Render(waiting, "/waiting", request(t, "/waiting"), ssr.Hosts{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	cancel()
	<-done
}

func TestHostConcurrencyIsBoundedAndQueuedCallsProgress(t *testing.T) {
	source := strings.Replace(parallelBundle, "['amber', 'birch', 'cobalt']", "Array.from({length:96}, (_,i) => String(i))", 1)
	engine, err := ssr.New("bounded.js", []byte(source), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var running, peak atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, calls, err := engine.Render(ctx, "/bounded", request(t, "/bounded"), ssr.Hosts{Remote: func(_ context.Context, id, _ string) ([]byte, error) {
		n := running.Add(1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		running.Add(-1)
		return answer(id), nil
	}})
	if err != nil || len(calls) != 96 || !strings.HasSuffix(result.Body, "94,95;/bounded") {
		t.Fatalf("%+v calls=%d: %v", result, len(calls), err)
	}
	if peak.Load() > 32 || peak.Load() < 2 {
		t.Fatalf("peak concurrency %d", peak.Load())
	}
}

func TestHostPanicBecomesRenderFailure(t *testing.T) {
	engine, err := ssr.New("parallel.js", []byte(parallelBundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = engine.Render(context.Background(), "/panic", request(t, "/panic"), ssr.Hosts{Remote: func(context.Context, string, string) ([]byte, error) { panic("host exploded") }})
	if err == nil || !strings.Contains(fmt.Sprint(err), "host exploded") {
		t.Fatal(err)
	}
}

func TestCancellationInterruptsJavaScriptAndReplacesRuntime(t *testing.T) {
	source := `globalThis.__skgo_ping = () => 'ok'; globalThis.__skgo_render = json => {
   if (JSON.parse(json).route_id === '/spin') { while (true) {} }
   return {done:true, body:'fresh page'};
 };`
	engine, err := ssr.New("spin.js", []byte(source), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err = engine.Render(ctx, "/spin", request(t, "/spin"), ssr.Hosts{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("spinning render: %v", err)
	}
	result, _, err := engine.Render(context.Background(), "/fresh", request(t, "/fresh"), ssr.Hosts{})
	if err != nil || result.Body != "fresh page" {
		t.Fatalf("%+v: %v", result, err)
	}
}

func TestFetchAndRemoteCallsShareTheAsyncScheduler(t *testing.T) {
	source := `globalThis.__skgo_ping = () => 'ok'; globalThis.__skgo_render = () => {
   const result={done:false};
   Promise.all([__skgo_remote('query',''),__skgo_fetch('one'),__skgo_fetch('two')]).then(
     values=>{result.body=values.join('|');result.done=true},
     error=>{result.failure=String(error);result.done=true});
   return result;
 };`
	engine, err := ssr.New("fetch.js", []byte(source), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	gate := make(chan struct{})
	var started atomic.Int32
	call := func(ctx context.Context, value string) ([]byte, error) {
		if started.Add(1) == 3 {
			close(gate)
		}
		select {
		case <-gate:
			return []byte(value), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	result, _, err := engine.Render(ctx, "/fetch", request(t, "/fetch"), ssr.Hosts{
		Remote: func(ctx context.Context, id, _ string) ([]byte, error) { return call(ctx, id) },
		Fetch:  func(ctx context.Context, payload []byte) ([]byte, error) { return call(ctx, string(payload)) },
	})
	if err != nil || result.Body != "query|one|two" {
		t.Fatalf("%+v: %v", result, err)
	}
}
