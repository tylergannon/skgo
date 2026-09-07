package ssrspike

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// The spike's inputs. Everything the tests expect is written down here or in
// fixtures.json — never read back out of the thing under test.
func spikeDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "ephemeral", "spike-ssr")
}

type fixtures struct {
	Remotes  map[string]Answer          `json:"remotes"`
	Requests map[string]json.RawMessage `json:"requests"`
}

func load(t *testing.T) (*goja0Program, fixtures) {
	t.Helper()
	dir := spikeDir(t)

	bundle := filepath.Join(dir, "dist", "ssr.js")
	if _, err := os.Stat(bundle); err != nil {
		t.Fatalf("no SSR bundle at %s — build it first:\n\tjust build && (cd ephemeral/spike-ssr && node build.mjs)\n%v", bundle, err)
	}
	prog, err := Program(bundle)
	if err != nil {
		t.Fatalf("goja could not compile the bundle: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return &goja0Program{prog}, f
}

// answerer serves the fixture table and shouts if the engine asks for a key
// nobody wrote down. An unexpected key is a finding, not a default.
func answerer(t *testing.T, f fixtures) RemoteFunc {
	t.Helper()
	return func(id, payload string) Answer {
		key := id + "/" + payload
		a, ok := f.Remotes[key]
		if !ok {
			t.Errorf("render asked Go for remote key %q, which is not in fixtures.json", key)
			return Answer{Error: &AnswerError{Status: 500, Message: "no fixture for " + key}}
		}
		return a
	}
}

func render(t *testing.T, p *goja0Program, f fixtures, name string) (*Engine, Result) {
	t.Helper()
	req, ok := f.Requests[name]
	if !ok {
		t.Fatalf("no request fixture named %q", name)
	}
	e, err := New(p.p)
	if err != nil {
		t.Fatalf("loading the bundle into goja: %v", err)
	}
	r, err := e.Render(req, answerer(t, f))
	if err != nil {
		t.Fatalf("render %q: %v", name, err)
	}
	if !r.Done {
		t.Fatalf("render %q never settled: goja returned from __skgo_render with the promise still pending, which means the bundle needs an event loop", name)
	}
	if r.Error != "" {
		t.Fatalf("render %q threw:\n%s", name, r.Error)
	}
	if testing.Verbose() {
		t.Logf("---- %s ----\nHEAD: %s\nBODY: %s\n", name, r.Head, r.Body)
	}
	return e, r
}

func mustContain(t *testing.T, name, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s: rendered HTML does not contain %q\n---\n%s\n---", name, w, body)
		}
	}
}

// TestRemoteFunctionAnsweredByGo is the crux: a page whose markup awaits two
// remote functions, rendered inside goja, with Go supplying both answers in
// process. The expected strings are the ones the fixture file supplied, so a
// render that invented its own content cannot pass.
func TestRemoteFunctionAnsweredByGo(t *testing.T) {
	p, f := load(t)
	e, r := render(t, p, f, "probe")

	mustContain(t, "probe", r.Body,
		`<h1 data-testid="title">SSR probe</h1>`,
		`<p data-testid="site-name">Anvil and Ampersand</p>`,
		`<p data-testid="colocated">site.remote.go answered this</p>`,
		`<li data-testid="todo">Recalibrate the flux capacitor</li>`,
		`<li data-testid="todo" class="private">Alphabetise the spice rack</li>`,
	)

	// The engine must actually have come back out to Go, under kit's key shape:
	// `hash(file)` + '/' + name + '/' + stringify_remote_arg(arg).
	// (kit/src/exports/vite/index.js:678 and kit/src/runtime/shared.js
	// `create_remote_key`.) Both queries here take no argument, so the payload
	// is the empty string and the key ends in a trailing slash.
	calls := strings.Join(e.Calls(), " ")
	for _, want := range []string{"ezl04l/getSite/", "16h93j5/getTodos/"} {
		if !strings.Contains(calls, want) {
			t.Errorf("Go was never asked for %q; it was asked for %v", want, e.Calls())
		}
	}

	// ...and the values must be recorded for hydration under the same key, or
	// the browser will refetch everything the server just rendered.
	q := r.Data["q"]
	if q == nil {
		t.Fatalf("no query data collected for hydration; got %v", r.Data)
	}
	for _, want := range []string{"ezl04l/getSite/", "16h93j5/getTodos/"} {
		if _, ok := q[want]; !ok {
			t.Errorf("hydration payload has no entry for %q; keys are %v", want, keys(q))
		}
	}
	var site struct {
		Name      string `json:"name"`
		Colocated string `json:"colocated"`
	}
	if err := json.Unmarshal(q["ezl04l/getSite/"].Value, &site); err != nil {
		t.Fatal(err)
	}
	if site.Name != "Anvil and Ampersand" {
		t.Errorf("hydration payload carries site name %q, want %q", site.Name, "Anvil and Ampersand")
	}
}

// TestServerLoadData renders /account, whose two `+*.server.ts` are Go's. The
// per-node data is supplied the way a Go load would supply it, and kit's own
// cumulative merge (render.js:167-183) has to put the layout's fields within
// reach of the page.
func TestServerLoadData(t *testing.T) {
	p, f := load(t)
	_, r := render(t, p, f, "account")

	mustContain(t, "account", r.Body,
		`<p data-testid="account-user">Account of Marguerite Okonkwo</p>`,
		`<p data-testid="account-serial">90210</p>`,
		`<p data-testid="parent-user">The layout loaded Marguerite Okonkwo (via the Go layout load)</p>`,
		`<h1 data-testid="title">Overview</h1>`,
	)
	if strings.Contains(r.Body, "implemented in Go") {
		t.Error("the generated +page.server.ts stub ran; kit must never call it")
	}
}

// TestErrorPage renders the branch kit builds in respond_with_error: the root
// layout plus node 1, the root `+error.svelte`, with `page.status` and
// `page.error` coming from `$app/state`, which reads the `__request__` context
// entry render.js:190 puts in the render options.
func TestErrorPage(t *testing.T) {
	p, f := load(t)
	_, r := render(t, p, f, "error")

	mustContain(t, "error", r.Body,
		`<h1 data-testid="title">Error 503</h1>`,
		`<p data-testid="error-message">the ledger service is having a lie-down</p>`,
	)
	if strings.Contains(r.Body, "Overview") {
		t.Error("the error branch rendered a page component")
	}
}

// TestBoundaryWithPendingIsNotServerRendered pins down why the example app's
// own home page cannot demonstrate SSR of a remote function. Svelte's server
// compiler drops the children of a `<svelte:boundary>` that has a `pending`
// snippet and emits only the pending block
// (svelte/src/compiler/phases/3-transform/server/visitors/SvelteBoundary.js:59-74).
// It is a property of the app's markup, not of the engine.
func TestBoundaryWithPendingIsNotServerRendered(t *testing.T) {
	p, f := load(t)
	e, r := render(t, p, f, "home")

	mustContain(t, "home", r.Body,
		`<h1 data-testid="title">Home</h1>`,
		`<p data-testid="greeting">Hello, skgo! This is Svelte, served by Go.</p>`,
		`<p data-testid="site-pending">loading…</p>`,
	)
	if strings.Contains(r.Body, "Anvil and Ampersand") {
		t.Error("the boundary's children rendered after all — this test's premise is stale, re-read SvelteBoundary.js")
	}
	if n := len(e.Calls()); n != 0 {
		t.Errorf("Go was asked for %d remote functions on a page that server-renders none: %v", n, e.Calls())
	}
}

// TestRunPerRuntimeIsIsolated: a pool of Runtimes, each rendering a different
// page at the same time, is the shape a Go server would actually use.
func TestRunPerRuntimeIsIsolated(t *testing.T) {
	p, f := load(t)

	names := []string{"probe", "account", "error", "probe", "account", "error", "probe", "account"}
	type out struct {
		name string
		body string
	}
	results := make(chan out, len(names))
	for _, name := range names {
		go func(name string) {
			e, err := New(p.p)
			if err != nil {
				t.Errorf("New: %v", err)
				results <- out{name, ""}
				return
			}
			r, err := e.Render(f.Requests[name], func(id, payload string) Answer {
				return f.Remotes[id+"/"+payload]
			})
			if err != nil {
				t.Errorf("Render(%s): %v", name, err)
			}
			results <- out{name, r.Body}
		}(name)
	}

	want := map[string]string{
		"probe":   `<p data-testid="site-name">Anvil and Ampersand</p>`,
		"account": `<p data-testid="account-serial">90210</p>`,
		"error":   `<h1 data-testid="title">Error 503</h1>`,
	}
	for range names {
		got := <-results
		if !strings.Contains(got.body, want[got.name]) {
			t.Errorf("%s rendered by a pooled Runtime is missing %q:\n%s", got.name, want[got.name], got.body)
		}
	}
}

// TestReentrantRenderOnOneRuntime is the adversarial case for the
// AsyncLocalStorage shim. A Go host function is a re-entry point: while render
// A is suspended waiting for Go to answer a remote, Go could start render B on
// the same Runtime. The shim keeps one store per AsyncLocalStorage instance and
// never restores it, so B's store replaces A's. This test records what actually
// happens rather than asserting a hoped-for outcome.
func TestReentrantRenderOnOneRuntime(t *testing.T) {
	p, f := load(t)
	e, err := New(p.p)
	if err != nil {
		t.Fatal(err)
	}

	var inner Result
	var innerErr error
	outer, err := e.Render(f.Requests["probe"], func(id, payload string) Answer {
		if id == "ezl04l/getSite" && inner.Body == "" && innerErr == nil {
			// Re-enter the same Runtime mid-render, exactly as a Go handler that
			// decided to serve a second request on this engine would.
			inner, innerErr = e.Render(f.Requests["account"], func(id, payload string) Answer {
				return f.Remotes[id+"/"+payload]
			})
		}
		return f.Remotes[id+"/"+payload]
	})

	t.Logf("outer render: err=%v done=%v error=%q", err, outer.Done, outer.Error)
	t.Logf("inner render: err=%v done=%v error=%q", innerErr, inner.Done, inner.Error)
	t.Logf("outer body: %s", outer.Body)
	t.Logf("inner body: %s", inner.Body)

	outerOK := err == nil && outer.Done && outer.Error == "" &&
		strings.Contains(outer.Body, `<p data-testid="site-name">Anvil and Ampersand</p>`)
	innerOK := innerErr == nil && inner.Done && inner.Error == "" &&
		strings.Contains(inner.Body, `<p data-testid="account-serial">90210</p>`)

	switch {
	case outerOK && innerOK:
		t.Log("VERDICT: one Runtime survived two interleaved renders.")
	case outerOK && !innerOK:
		// This is what actually happens. goja has no event loop: the promise job
		// queue is drained when the OUTERMOST call returns, so the nested render's
		// continuations are still queued when `Render` reads its result. Go gets an
		// unsettled, empty page and no error at all.
		t.Log("VERDICT: the outer render survived, but the nested one came back " +
			"unsettled and empty, with no error. A Runtime must render exactly one " +
			"page at a time; concurrency needs a pool of Runtimes.")
	default:
		t.Log("VERDICT: re-entering the Runtime mid-render corrupted the outer render.")
	}

	if outerOK && !innerOK && inner.Body != "" {
		t.Errorf("the nested render produced partial output %q; the failure mode this "+
			"test documents (empty and unsettled) has changed", inner.Body)
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// goja0Program keeps the goja import out of the test's signature noise.
type goja0Program struct{ p *goja.Program }
