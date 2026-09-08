package ssr_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/tylergannon/skgo/internal/ssr"
)

// bundle is a stand-in for the real SSR bundle: the same two entry points, and
// nothing else. It keeps the page it is rendering in a module global and reads
// it back after a trip out to Go, which is exactly the shape Svelte's
// no-AsyncLocalStorage render context has — and exactly what a second render on
// the same runtime would destroy.
const bundle = `
var current = null;
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	current = req.route_id;
	var result = { done: false };
	globalThis.__skgo_remote('fixture/get', req.route_id).then(function(raw) {
	var answer = JSON.parse(raw);
	Object.assign(result, {
		done: true,
		failure: '',
		redirect: null,
		status: req.status,
		error: req.error,
		head: '',
		body: current + ':' + answer.v
	});
	}, function(e) { result.failure = String(e); result.done = true; });
	return result;
};
`

// neverSettles is a bundle whose render is still pending when the host gets
// control back. It is what a re-entrant render on one runtime looks like from
// Go: a result with no error in it and nothing filled in.
const neverSettles = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function () {
	var result = { done: false, failure: '', redirect: null, status: 200, error: null, head: '', body: '' };
	new Promise(function () {}).then(function () { result.done = true; });
	return result;
};
`

func request(t *testing.T, routeID string) []byte {
	t.Helper()
	raw, err := json.Marshal(ssr.Request{URL: "http://example.test/", RouteID: routeID})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func answer(value string) []byte {
	raw, _ := json.Marshal(map[string]any{"v": value})
	return raw
}

// TestConcurrentRendersDoNotShareARuntime holds two renders open at the same
// time, from inside the host call each of them makes, and then lets both
// finish. On one runtime the second render's module state would have replaced
// the first's, and the first would come back describing the second's page.
func TestConcurrentRendersDoNotShareARuntime(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(bundle), 4, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Neither render may leave its host call until both have entered one, so
	// the two renders provably overlap.
	both := make(chan struct{}, 2)
	release := make(chan struct{})

	results := make([]string, 2)
	routes := []string{"/first", "/second"}
	var wg sync.WaitGroup
	for i, route := range routes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := engine.Render(context.Background(), route, request(t, route), ssr.Hosts{Remote: func(_ context.Context, id, payload string) ([]byte, error) {
				both <- struct{}{}
				<-release
				return answer("answered " + payload), nil
			}})
			if err != nil {
				t.Errorf("%s: %v", route, err)
				return
			}
			results[i] = result.Body
		}()
	}

	<-both
	<-both
	close(release)
	wg.Wait()

	for i, route := range routes {
		want := route + ":answered " + route
		if results[i] != want {
			t.Errorf("render of %s produced %q, want %q", route, results[i], want)
		}
	}
	if created := engine.Created(); created < 2 {
		t.Errorf("two overlapping renders were served by %d runtime(s)", created)
	}
}

// TestARenderThatNeverFinishesIsAnError pins the one failure that arrives with
// no error attached. Nothing may read a body out of a result whose promise is
// still pending: it is empty, and an empty document is not a page.
func TestARenderThatNeverFinishesIsAnError(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(neverSettles), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = engine.Render(context.Background(), "/", request(t, "/"), ssr.Hosts{Remote: func(context.Context, string, string) ([]byte, error) {
		return answer(""), nil
	}})
	if err == nil {
		t.Fatal("a render that did not finish was reported as a success")
	}
	if !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("error was %v", err)
	}
}

// TestAHostFailureIsAGoError keeps a failed remote answer from being swallowed
// by the component boundary that catches the throw it turns into.
func TestAHostFailureIsAGoError(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(bundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, calls, err := engine.Render(context.Background(), "/", request(t, "/"), ssr.Hosts{Remote: func(context.Context, string, string) ([]byte, error) {
		return nil, errNoAnswer
	}})
	if err == nil {
		t.Fatal("a render whose host call failed was reported as a success")
	}
	if len(calls) != 1 || calls[0].ID != "fixture/get" {
		t.Errorf("calls = %v, want one call to fixture/get", calls)
	}
}

var errNoAnswer = &hostError{}

type hostError struct{}

func (*hostError) Error() string { return "no answer" }

// TestABundleTheEngineCannotRunIsRefusedAtStartup keeps a broken bundle from
// becoming a broken page.
func TestABundleTheEngineCannotRunIsRefusedAtStartup(t *testing.T) {
	for name, source := range map[string]string{
		"does not parse":       "function (",
		"defines no entry":     "globalThis.__skgo_ping = function () { return 'ok'; };",
		"does not come up":     "globalThis.__skgo_ping = function () { return 'no'; }; globalThis.__skgo_render = function () {};",
		"throws on evaluation": "throw new Error('boom');",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ssr.New("bundle.js", []byte(source), 1, nil); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

// TestTheEngineReusesItsRuntimes checks that a pool is a pool: renders that do
// not overlap are served by runtimes that already exist.
func TestTheEngineReusesItsRuntimes(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(bundle), 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, _, err := engine.Render(context.Background(), "/", request(t, "/"), ssr.Hosts{Remote: func(_ context.Context, _, payload string) ([]byte, error) {
			return answer(payload), nil
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if created := engine.Created(); created != 1 {
		t.Errorf("twenty sequential renders built %d runtimes, want 1", created)
	}
}

// fetching is a stand-in bundle that does what the real one's `event.fetch`
// polyfill does: build a JSON envelope and hand it to `__skgo_fetch`, then
// read back a response or throw the answer's error. It is the proof that the
// engine's fetch never does its own I/O — everything it "does" is call this
// one function and report what came back.
const fetching = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	var result = { done: false };
	globalThis.__skgo_fetch(JSON.stringify({ method: 'GET', url: req.url + 'greeting' })).then(function(raw) {
	var answer = JSON.parse(raw);
	var body = answer.error ? 'error:' + answer.error : answer.response.status + ':' + answer.response.body;
	Object.assign(result, { done: true, failure: '', redirect: null, status: 200, error: null, head: '', body: body });
	}, function(e) { result.failure = String(e); result.done = true; });
	return result;
};
`

// matching is a stand-in bundle that does what the real $app/paths wrapper's
// match() does: hand a pathname to __skgo_match and report the route it
// found, or "null" for one that matched nothing.
const matching = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	var raw = globalThis.__skgo_match(req.route_id);
	return { done: true, failure: '', redirect: null, status: 200, error: null, head: '', body: raw };
};
`

// TestFetchIsACallBackIntoGoNeverASocket is the engine-level half of the
// fetch proof: a render's `event.fetch`, exercised exactly the way the real
// polyfill exercises it — a JSON envelope handed to `__skgo_fetch` — comes
// back as the answer Go gave, with nothing in between that could have opened
// a connection of its own.
func TestFetchIsACallBackIntoGoNeverASocket(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(fetching), 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	var got ssr.FetchRequest
	result, _, err := engine.Render(context.Background(), "/", request(t, "/"), ssr.Hosts{
		Fetch: func(_ context.Context, payload []byte) ([]byte, error) {
			if err := json.Unmarshal(payload, &got); err != nil {
				t.Fatal(err)
			}
			return json.Marshal(ssr.FetchAnswer{
				Response: &ssr.FetchResponse{Status: 200, Body: `{"message":"hello from Go"}`},
			})
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got.Method != "GET" || got.URL != "http://example.test/greeting" {
		t.Errorf("Go saw %+v, want GET http://example.test/greeting", got)
	}
	if want := `200:{"message":"hello from Go"}`; result.Body != want {
		t.Errorf("body = %q, want %q", result.Body, want)
	}
}

// TestFetchWithNoHostRefuses is the engine-level half of the refusal: a
// render that calls fetch with nothing answering it gets a clear error rather
// than a ReferenceError, the same shape a remote call with no host gets.
func TestFetchWithNoHostRefuses(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(fetching), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.Render(context.Background(), "/", request(t, "/"), ssr.Hosts{}); err == nil {
		t.Fatal("a render whose fetch had nothing to answer it was reported as a success")
	}
}

// TestMatchIsACallBackIntoGo is the engine-level half of the match proof: a
// render's `$app/paths` `match()`, exercised the way the real wrapper
// exercises it — a pathname handed to `__skgo_match` — comes back with the
// route Go's own table found, and "null" for one that matched nothing.
func TestMatchIsACallBackIntoGo(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(matching), 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, _, err := engine.Render(context.Background(), "/api/todos", request(t, "/api/todos"), ssr.Hosts{
		Match: func(pathname string) (string, map[string]string, bool) {
			if pathname == "/api/todos" {
				return "/api/todos", map[string]string{}, true
			}
			return "", nil, false
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := `{"id":"/api/todos","params":{}}`; result.Body != want {
		t.Errorf("body = %q, want %q", result.Body, want)
	}

	result, _, err = engine.Render(context.Background(), "/no-such-route", request(t, "/no-such-route"), ssr.Hosts{
		Match: func(string) (string, map[string]string, bool) { return "", nil, false },
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Body != "null" {
		t.Errorf("body = %q, want null", result.Body)
	}
}

// reporting is a bundle that reports back the three things a document's status
// depends on: the status the page ended on, the error it ended with, and a
// redirect thrown while it rendered.
const reporting = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	if (req.route_id === '/go-away') {
		return {
			done: true,
			failure: '',
			redirect: { status: 307, location: '/elsewhere' },
			status: 200,
			error: null,
			head: '',
			body: ''
		};
	}
	return {
		done: true,
		failure: '',
		redirect: null,
		status: 418,
		error: { status: 418, message: 'I am a teapot' },
		head: '',
		body: 'brewed'
	};
};
`

// TestARenderReportsTheStatusAndErrorItEndedWith is what makes the response
// status the one kit would give. `transformError` runs *during* the render, so
// a boundary that catches something changes both after Go has already handed
// the request over; a result that did not carry them back would answer a caught
// error with 200.
func TestARenderReportsTheStatusAndErrorItEndedWith(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(reporting), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.Render(context.Background(), "/teapot", request(t, "/teapot"), ssr.Hosts{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != 418 {
		t.Errorf("status = %d, want 418", result.Status)
	}
	if result.Error == nil || result.Error.Message != "I am a teapot" || result.Error.Status != 418 {
		t.Errorf("error = %+v, want a 418 teapot", result.Error)
	}
	if result.Redirect != nil {
		t.Errorf("redirect = %+v, want none", result.Redirect)
	}
	if result.Body != "brewed" {
		t.Errorf("body = %q", result.Body)
	}
}

// TestARedirectThrownDuringARenderIsNotAFailure keeps a redirect out of the
// error path. Kit answers one with a bare 3xx and no document at all, so a
// render that ends in a redirect must not look like a render that broke.
func TestARedirectThrownDuringARenderIsNotAFailure(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(reporting), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.Render(context.Background(), "/go-away", request(t, "/go-away"), ssr.Hosts{})
	if err != nil {
		t.Fatalf("a redirect was reported as a failure: %v", err)
	}
	if result.Redirect == nil || result.Redirect.Status != 307 || result.Redirect.Location != "/elsewhere" {
		t.Fatalf("redirect = %+v, want 307 to /elsewhere", result.Redirect)
	}
}

// aPageErrorBundle echoes the request's error straight back as the render's
// own, and folds one of its properties into the body, so the test can tell
// whether a property `Request.Error` carries beyond status and message
// actually reached the +error.svelte component's props — not merely that Go
// serialized it — and that it survived the trip back out through
// `Result.Error` too.
const aPageErrorBundle = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	return {
		done: true,
		failure: '',
		redirect: null,
		status: req.status,
		error: req.error,
		head: '',
		body: 'support id seen by the component: ' + (req.error ? req.error.supportId : '(none)')
	};
};
`

// TestAnExtraAppErrorFieldReachesTheComponentAndSurvivesTheRoundTrip is the
// host-binding half of the handleError hook: kit's App.Error is not fixed to
// status and message, an app may augment it with fields of its own, and
// those fields have to cross into the engine exactly like any other request
// data and come back out again with Result.Error.
func TestAnExtraAppErrorFieldReachesTheComponentAndSurvivesTheRoundTrip(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(aPageErrorBundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(ssr.Request{
		URL:     "http://example.test/",
		RouteID: "/error/unexpected",
		Status:  500,
		Error:   &ssr.Error{Status: 500, Message: "Something went wrong on our end.", Extra: map[string]any{"supportId": "case-1121"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.Render(context.Background(), "/error/unexpected", raw, ssr.Hosts{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Body != "support id seen by the component: case-1121" {
		t.Errorf("body = %q, the component never saw the extra field", result.Body)
	}
	if result.Error == nil || result.Error.Status != 500 || result.Error.Message != "Something went wrong on our end." {
		t.Fatalf("error = %+v, want the same status and message sent in", result.Error)
	}
	if got := result.Error.Extra["supportId"]; got != "case-1121" {
		t.Errorf("Extra[supportId] = %v, want it to have round-tripped through Result.Error too", got)
	}
}

// A render that reports through `console` reaches Go. The engine is a bare
// ECMAScript runtime and goja has no console at all, so before there was one
// every such report was a ReferenceError thrown in the middle of a render —
// which is how kit's `log_handle_error_hook_failure` and Svelte's own warnings
// disappeared. The route comes with it, because a line an operator reads is
// worth little without the page that wrote it.
func TestTheEngineConsoleReachesGo(t *testing.T) {
	const reporting = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	console.error(new Error('the render fell over'));
	console.log('and this is just chatter');
	return { done: true, failure: '', redirect: null, status: 200, error: null, head: '', body: '' };
};
`
	var mu sync.Mutex
	var lines []string
	engine, err := ssr.New("bundle.js", []byte(reporting), 1, func(routeID, level, text string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, routeID+" "+level+" "+text)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.Render(context.Background(), "/checkout", request(t, "/checkout"), ssr.Hosts{}); err != nil {
		t.Fatalf("render: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 2 {
		t.Fatalf("got %d console lines, want 2: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "/checkout error Error: the render fell over") {
		t.Errorf("console.error reached Go as %q", lines[0])
	}
	if lines[1] != "/checkout log and this is just chatter" {
		t.Errorf("console.log reached Go as %q", lines[1])
	}
}

// The two globals goja is missing that kit's own runtime declares outright:
// `Promise.withResolvers`, which its streaming helper calls, and
// `Symbol.asyncIterator`, which its live-query iterators use as a method name.
// Without the symbol that method is called "undefined" instead, which is not an
// error anywhere — it is just wrong later.
func TestTheEngineHasTheGlobalsKitAssumes(t *testing.T) {
	const probing = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function () {
	var d = Promise.withResolvers();
	d.resolve('resolved');
	var stream = { [Symbol.asyncIterator]() { return 'iterable'; } };
	return {
		done: true,
		failure: '',
		redirect: null,
		status: 200,
		error: null,
		head: '',
		body: typeof Symbol.asyncIterator + ' ' + typeof d.promise.then + ' ' + stream[Symbol.asyncIterator]()
	};
};
`
	engine, err := ssr.New("bundle.js", []byte(probing), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := engine.Render(context.Background(), "/", request(t, "/"), ssr.Hosts{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := "symbol function iterable"; result.Body != want {
		t.Errorf("body = %q, want %q", result.Body, want)
	}
}

// deferred is a bundle whose render finishes only after a macrotask runs. It is
// the shape kit's `query.batch` has: the calls a render made are collected and
// flushed from a `setTimeout(..., 0)`, deliberately a macrotask so that
// everything awaited in the same turn ends up in one batch.
const deferred = `
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	var result = { done: false, failure: '', redirect: null, status: 200, error: null, head: '', body: '' };
	var collected = [];
	// Two calls in the same turn, one flush: exactly what a batch query does.
	collected.push(req.route_id + '/a');
	collected.push(req.route_id + '/b');
	setTimeout(function () {
		globalThis.__skgo_remote('fixture/batch', collected.join(',')).then(function(raw) {
		var answer = JSON.parse(raw);
		result.body = answer.v;
		result.done = true;
		});
	}, 0);
	return result;
};
`

// abandoned is a bundle that finishes without ever waiting for the macrotask it
// scheduled — a component that reads `.loading` on a batch query and never
// awaits it. The callback is left on the queue, and the runtime goes back to
// the pool with it.
const abandoned = `
var ran = [];
globalThis.__skgo_ping = function () { return 'ok'; };
globalThis.__skgo_render = function (json) {
	var req = JSON.parse(json);
	if (req.route_id === '/abandon') {
		setTimeout(function () { ran.push('stale'); }, 0);
		return { done: true, failure: '', redirect: null, status: 200, error: null, head: '', body: 'abandoned' };
	}
	// The next page waits on a macrotask of its own, which is the only moment
	// the queue is ever pumped — and so the only moment a callback left over
	// from the last render could run.
	var result = { done: false, failure: '', redirect: null, status: 200, error: null, head: '', body: '' };
	setTimeout(function () {
		result.body = 'ran:' + ran.join(',');
		result.done = true;
	}, 0);
	return result;
};
`

// A render that finishes only after its macrotask runs still finishes: the
// engine drains the queue between turns of the microtask queue, which is
// exactly when a real `setTimeout(fn, 0)` would fire.
func TestARenderIsDrivenPastItsMacrotasks(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(deferred), 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	var asked []string
	result, _, err := engine.Render(context.Background(), "/batch", request(t, "/batch"), ssr.Hosts{Remote: func(_ context.Context, id, payload string) ([]byte, error) {
		asked = append(asked, id+" "+payload)
		return answer("answered " + payload), nil
	}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !result.Done {
		t.Fatal("the render did not finish")
	}
	// One call carrying both, rather than one call each: the macrotask is what
	// makes a batch a batch, so a queue that ran too early would show two.
	if len(asked) != 1 {
		t.Fatalf("the render made %d host calls, want 1: %v", len(asked), asked)
	}
	if asked[0] != "fixture/batch /batch/a,/batch/b" {
		t.Errorf("host call = %q", asked[0])
	}
	if result.Body != "answered /batch/a,/batch/b" {
		t.Errorf("body = %q", result.Body)
	}
}

// A callback the last render scheduled and never waited for belongs to a
// request that is over. The next render on that runtime must not run it: it
// would execute against the new render's state, and whatever it produced would
// be recorded against the wrong page.
func TestWorkOneRenderAbandonedDoesNotRunInTheNext(t *testing.T) {
	engine, err := ssr.New("bundle.js", []byte(abandoned), 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	host := ssr.Hosts{Remote: func(_ context.Context, id, payload string) ([]byte, error) { return answer(payload), nil }}

	first, _, err := engine.Render(context.Background(), "/abandon", request(t, "/abandon"), host)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	if first.Body != "abandoned" {
		t.Fatalf("first body = %q", first.Body)
	}

	second, _, err := engine.Render(context.Background(), "/after", request(t, "/after"), host)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if second.Body != "ran:" {
		t.Errorf("body = %q, want ran: — the abandoned callback ran inside the next render", second.Body)
	}
}
