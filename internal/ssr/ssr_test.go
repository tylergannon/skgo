package ssr_test

import (
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
	var answer = JSON.parse(globalThis.__skgo_remote('fixture/get', req.route_id));
	return {
		done: true,
		failure: '',
		redirect: null,
		status: req.status,
		error: req.error,
		head: '',
		body: current + ':' + answer.v
	};
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
			result, _, err := engine.Render(route, request(t, route), func(id, payload string) ([]byte, error) {
				both <- struct{}{}
				<-release
				return answer("answered " + payload), nil
			})
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
	_, _, err = engine.Render("/", request(t, "/"), func(string, string) ([]byte, error) {
		return answer(""), nil
	})
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
	_, calls, err := engine.Render("/", request(t, "/"), func(string, string) ([]byte, error) {
		return nil, errNoAnswer
	})
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
		if _, _, err := engine.Render("/", request(t, "/"), func(_, payload string) ([]byte, error) {
			return answer(payload), nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if created := engine.Created(); created != 1 {
		t.Errorf("twenty sequential renders built %d runtimes, want 1", created)
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
	result, _, err := engine.Render("/teapot", request(t, "/teapot"), nil)
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
	result, _, err := engine.Render("/go-away", request(t, "/go-away"), nil)
	if err != nil {
		t.Fatalf("a redirect was reported as a failure: %v", err)
	}
	if result.Redirect == nil || result.Redirect.Status != 307 || result.Redirect.Location != "/elsewhere" {
		t.Fatalf("redirect = %+v, want 307 to /elsewhere", result.Redirect)
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
	if _, _, err := engine.Render("/checkout", request(t, "/checkout"), nil); err != nil {
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
	result, _, err := engine.Render("/", request(t, "/"), nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := "symbol function iterable"; result.Body != want {
		t.Errorf("body = %q, want %q", result.Body, want)
	}
}
