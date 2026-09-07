// Package ssr runs SvelteKit's own server renderer inside the Go process.
//
// The JavaScript it executes is the bundle the skgo adapter built: kit's `Root`
// component, kit's `Props`/`RenderNode`, kit's remote-function wrappers and
// Svelte's server renderer, with the app's own components. Nothing in it reads
// a file, opens a socket or sets a timer — every remote function's body is a
// call back out to Go — so the engine needs no host beyond a handful of web
// globals the bundle carries itself.
//
// One runtime renders one page at a time. That is not a tuning choice: Svelte's
// no-AsyncLocalStorage mode keeps its render context in a module global, and a
// render started while another is in flight on the same runtime silently
// returns an empty document. The pool is what makes concurrent requests safe,
// and Result.Done is what proves a render actually finished.
package ssr

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Host answers one remote-function call made during a render. id is
// `<hash>/<name>`, as kit's own vite plugin assigns it, and payload is
// `stringify_remote_arg(arg)`; together they are the key the browser's query
// cache looks the value up under. The returned bytes are the JSON envelope the
// bundle parses: `{"v": <value>}` or `{"e": {"status": n, "message": "..."}}`.
type Host func(id, payload string) ([]byte, error)

// Request is what a render is given. It is marshalled straight to the bundle's
// entry point, so the field names are the ones the entry reads.
type Request struct {
	// URL is the absolute URL of the page being rendered.
	URL string `json:"url"`
	// RouteID is kit's route id, or "" for a request that matched no route.
	RouteID string `json:"route_id"`
	// Params are the route's parameters.
	Params map[string]string `json:"params"`
	// Status is the status the document will be answered with.
	Status int `json:"status"`
	// Error is the error a page is rendering, or nil.
	Error *Error `json:"error"`
	// Form is the value of `page.form`.
	Form any `json:"form"`
	// Branch is the route's nodes, outermost first, with the data each one's
	// load produced.
	Branch []Node `json:"branch"`
	// ErrorComponents is one node index per branch slot whose `+error.svelte`
	// should catch a throw from that slot, or nil for a slot with none.
	ErrorComponents []*int `json:"error_components"`
	// Cookies are the request's cookies, by name.
	Cookies map[string]string `json:"cookies"`
	// ClientAddress is what `getClientAddress()` returns.
	ClientAddress string `json:"client_address"`
}

// Node is one slot of a route's branch.
type Node struct {
	// Index is the node's index in the manifest's node table.
	Index int `json:"node"`
	// Data is what the node's server load returned, already reduced to the
	// plain tree JSON carries. It is nil for a node with no load.
	Data any `json:"data"`
}

// Error is kit's `App.Error`.
type Error struct {
	Status  int    `json:"status,omitempty"`
	Message string `json:"message"`
}

// Result is one render.
type Result struct {
	// Done reports that the render finished. A render that neither resolved
	// nor rejected leaves it false, which is the only symptom of a re-entrant
	// render on one runtime — there is no error to catch.
	Done bool
	// Err is the message a render that threw left behind.
	Err string
	// Redirect is set when the render threw a redirect rather than an error —
	// a remote function called from a component, say. Kit answers the whole
	// document with a bare 3xx when that happens (`page/index.js`, the
	// `Redirect` arm of render_page's catch), so this is not a failure.
	Redirect *Redirect
	// Status is the status the document should be answered with. It is the one
	// the caller asked for unless an error boundary caught something, in which
	// case kit's `transformError` has replaced it with the caught error's.
	Status int
	// Error is `page.error` as the render left it: the error the caller passed
	// in, or the one a boundary caught and transformError jsonified. It is
	// what the boot script's `error:` carries.
	Error *Error
	// Head is what the components put in `<svelte:head>`.
	Head string
	// Body is the rendered markup.
	Body string
}

// Redirect is a redirect thrown during a render.
type Redirect struct {
	Status   int    `json:"status"`
	Location string `json:"location"`
}

// Engine is a pool of runtimes sharing one compiled program.
type Engine struct {
	program *goja.Program
	// idle holds runtimes that are not rendering. A runtime is only ever
	// checked out by one goroutine, which is what keeps a render from
	// re-entering one.
	idle chan *runtime
	// permits bounds how many runtimes exist at once. Taking a permit is what
	// a caller waits on when every runtime is busy.
	permits chan struct{}

	mu      sync.Mutex
	created int
}

// runtime is one goja Runtime with the bundle evaluated in it.
type runtime struct {
	vm     *goja.Runtime
	render goja.Callable

	// host is the answer function of the render currently in flight. It is
	// written and read by the one goroutine holding the runtime.
	host Host
	// calls records the `<id>` and payload of every remote call the current
	// render made, in order.
	calls []Call
	// failed is set when a host call could not be answered, so that the
	// failure surfaces as a Go error rather than as a rendered error message.
	failed error
}

// Call is one remote-function call a render made.
type Call struct {
	ID      string
	Payload string
}

// New compiles the bundle and returns an engine that will create at most size
// runtimes. A size below one is one.
func New(name string, source []byte, size int) (*Engine, error) {
	program, err := goja.Compile(name, string(source), false)
	if err != nil {
		return nil, fmt.Errorf("skgo: the SSR bundle does not parse: %w", err)
	}
	if size < 1 {
		size = 1
	}
	e := &Engine{
		program: program,
		idle:    make(chan *runtime, size),
		permits: make(chan struct{}, size),
	}
	for range size {
		e.permits <- struct{}{}
	}
	// One runtime is built now rather than on the first request, so that a
	// bundle the engine cannot evaluate is a startup failure instead of a
	// broken page.
	rt, err := e.newRuntime()
	if err != nil {
		return nil, err
	}
	<-e.permits
	e.idle <- rt
	return e, nil
}

// Size is how many runtimes the engine may hold.
func (e *Engine) Size() int { return cap(e.idle) }

// Created is how many runtimes the engine has actually built. It is what a test
// reads to see that concurrent renders did not share one.
func (e *Engine) Created() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.created
}

func (e *Engine) newRuntime() (*runtime, error) {
	rt := &runtime{vm: goja.New()}

	if err := rt.vm.Set("__skgo_remote", func(id, payload string) (string, error) {
		rt.calls = append(rt.calls, Call{ID: id, Payload: payload})
		if rt.host == nil {
			err := fmt.Errorf("skgo: nothing is answering remote functions for this render (%s)", id)
			rt.failed = errors.Join(rt.failed, err)
			return "", err
		}
		answer, err := rt.host(id, payload)
		if err != nil {
			rt.failed = errors.Join(rt.failed, err)
			return "", err
		}
		return string(answer), nil
	}); err != nil {
		return nil, err
	}

	if _, err := rt.vm.RunProgram(e.program); err != nil {
		return nil, fmt.Errorf("skgo: evaluating the SSR bundle: %w", err)
	}

	// Kit and Svelte each install their AsyncLocalStorage from a `.then()` on a
	// dynamic import, so neither is in place until the job queue has been
	// drained once. goja drains it when the outermost call returns; this no-op
	// run is that call. Without it the first async component fails with kit's
	// "Could not get the request store" in the middle of a render.
	if _, err := rt.vm.RunString("void 0"); err != nil {
		return nil, err
	}

	ping, ok := goja.AssertFunction(rt.vm.Get("__skgo_ping"))
	if !ok {
		return nil, errors.New("skgo: the SSR bundle defines no __skgo_ping; it is not a bundle this build of skgo can run")
	}
	if v, err := ping(goja.Undefined()); err != nil || v.String() != "ok" {
		return nil, fmt.Errorf("skgo: the SSR bundle did not come up: %v", err)
	}

	render, ok := goja.AssertFunction(rt.vm.Get("__skgo_render"))
	if !ok {
		return nil, errors.New("skgo: the SSR bundle defines no __skgo_render")
	}
	rt.render = render

	e.mu.Lock()
	e.created++
	e.mu.Unlock()
	return rt, nil
}

// Render renders one page, answering every remote function it asks for with
// host. It returns the calls the render made, in order, so the caller can
// serialise exactly the answers it gave into the document.
func (e *Engine) Render(request []byte, host Host) (Result, []Call, error) {
	rt, err := e.acquire()
	if err != nil {
		return Result{}, nil, err
	}
	defer e.release(rt)

	rt.host = host
	rt.calls = nil
	rt.failed = nil
	defer func() { rt.host = nil }()

	v, err := rt.render(goja.Undefined(), rt.vm.ToValue(string(request)))
	if err != nil {
		return Result{}, rt.calls, fmt.Errorf("skgo: rendering: %w", err)
	}
	if rt.failed != nil {
		return Result{}, rt.calls, rt.failed
	}

	object := v.ToObject(rt.vm)
	result := Result{
		Done:   boolOf(object.Get("done")),
		Err:    stringOf(object.Get("failure")),
		Status: intOf(object.Get("status")),
		Head:   stringOf(object.Get("head")),
		Body:   stringOf(object.Get("body")),
	}
	if err := decodeInto(rt.vm, object.Get("redirect"), &result.Redirect); err != nil {
		return result, rt.calls, err
	}
	if err := decodeInto(rt.vm, object.Get("error"), &result.Error); err != nil {
		return result, rt.calls, err
	}
	if !result.Done {
		// The one failure with no error attached: a render that was started
		// from inside another one on this runtime comes back with its promise
		// still pending, because the job queue is only drained when the
		// outermost call returns.
		return result, rt.calls, errors.New("skgo: the render did not finish; nothing on this runtime resolved it")
	}
	if result.Err != "" {
		return result, rt.calls, fmt.Errorf("skgo: the page threw while rendering: %s", result.Err)
	}
	return result, rt.calls, nil
}

// stringOf, boolOf and intOf read a property that may be absent, without
// turning an absent one into the word "undefined" or a nil dereference.
func stringOf(v goja.Value) string {
	if absent(v) {
		return ""
	}
	return v.String()
}

func boolOf(v goja.Value) bool {
	if absent(v) {
		return false
	}
	return v.ToBoolean()
}

func intOf(v goja.Value) int {
	if absent(v) {
		return 0
	}
	return int(v.ToInteger())
}

func absent(v goja.Value) bool {
	return v == nil || goja.IsUndefined(v) || goja.IsNull(v)
}

// decodeInto reads a structured property back out of the runtime through JSON,
// which is the only shape both sides already agree on.
func decodeInto(vm *goja.Runtime, v goja.Value, into any) error {
	if absent(v) {
		return nil
	}
	raw, err := json.Marshal(v.Export())
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

// acquire takes an idle runtime, or builds one while the pool is below its
// size, or waits for one to come back.
func (e *Engine) acquire() (*runtime, error) {
	select {
	case rt := <-e.idle:
		return rt, nil
	default:
	}
	select {
	case rt := <-e.idle:
		return rt, nil
	case <-e.permits:
		rt, err := e.newRuntime()
		if err != nil {
			e.permits <- struct{}{}
			return nil, err
		}
		return rt, nil
	}
}

func (e *Engine) release(rt *runtime) { e.idle <- rt }
