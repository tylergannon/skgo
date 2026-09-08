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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/dop251/goja"
	"github.com/tylergannon/skgo/internal/vite"
)

// Host answers one remote-function call made during a render. id is
// `<hash>/<name>`, as kit's own vite plugin assigns it, and payload is
// `stringify_remote_arg(arg)`; together they are the key the browser's query
// cache looks the value up under. The returned bytes are the JSON envelope the
// bundle parses: `{"v": "<devalue>"}` or `{"e": {"status": n, "message": "..."}}`.
//
// `v` is a string rather than a value, and for the same reason Node.Data is:
// it is devalue's flat form, so the bundle can hand it to the app's own
// decoders and a custom type arrives in the render with its methods.
type Host func(ctx context.Context, id, payload string) ([]byte, error)

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
	//
	// It is null for a remote form. Kit's `handle_remote_form_post_internal`
	// answers with `{type: 'success', status, location}` and no `data`, so
	// `render.js` computes `form_value = null`; `page.form` is the classic
	// `+page.server.js` actions slot, which skgo has no equivalent of. A
	// remote submission travels in FormAction instead.
	Form any `json:"form"`
	// FormAction is a form submission that was posted without JavaScript, if
	// this render is answering one.
	FormAction *Form `json:"form_action,omitempty"`
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
	// CSP is this render's Svelte-facing csp option, kit's own
	// `csp.script_needs_nonce ? { nonce: csp.nonce } : { hash:
	// csp.script_needs_hash }` (`page/render.js:198`), passed to Svelte's
	// own `render(Root, options)` call. Go's csp.go decides both booleans
	// before the engine ever runs — they depend only on the build's
	// configured directives, never on what a render produces — so this is
	// always the same answer the boot script's own nonce/hash attribute
	// (assembled separately, in Go, after the render) will use. It governs
	// the one other inline `<script>` a rendered document can carry:
	// Svelte's own hydratable-async-block script
	// (`internal/server/renderer.js`'s `#hydratable_block`), emitted when a
	// component's own top-level `await` needs to hand the client a value it
	// already resolved during SSR.
	CSP RequestCSP `json:"csp"`
}

// RequestCSP is one render's Svelte-facing csp option. Svelte's own renderer
// checks `csp.nonce` first and falls back to `csp.hash` only when that is
// empty, so carrying both fields is harmless — but Nonce and Hash are never
// both meaningful at once, mirroring kit's own ternary: documentCSP's
// ScriptNeedsNonce and ScriptNeedsHash cannot both be true for the same
// request.
type RequestCSP struct {
	// Nonce is this request's nonce, set only when the build's csp needs
	// one. "" is Svelte's own default (`csp = { hash: false }`,
	// `internal/server/renderer.js`).
	Nonce string `json:"nonce,omitempty"`
	// Hash is kit's `csp.script_needs_hash`.
	Hash bool `json:"hash"`
}

// Node is one slot of a route's branch.
type Node struct {
	// Index is the node's index in the manifest's node table.
	Index int `json:"node"`
	// Data is what the node's server load returned, serialized exactly as it
	// is for `__data.json`: devalue's flat form, with the app's transport
	// encoders applied. The bundle parses it back with the app's own decoders,
	// so a component renders against the same instance the browser will hold —
	// a `Money` with its `format()`, not the object its fields travelled in.
	//
	// A plain tree would lose that: JSON has no way to say which class an
	// object belongs to, and a method call on the object that arrived instead
	// throws in the middle of a render.
	//
	// A value the load promised crosses as kit's own promise placeholder,
	// `["Promise", <id>]`, and the bundle reads it back with a `Promise`
	// reviver into a promise that never settles — so an `{#await}` over one
	// renders its pending branch and the value follows the document down as a
	// chunk. Kit hands its renderer the promise itself, which is the same
	// thing said in a language that has one.
	//
	// "" is a node with no load.
	Data string `json:"data"`
}

// Form is one non-enhanced form submission, on its way into the render.
//
// The engine puts Output into the request's remote cache under the form
// instance ID names, which is exactly what kit's own `form` wrapper does at the
// end of a submission (`runtime/app/server/remote/form.js`). From there the
// instance's `result`, `fields.<name>.issues()` and `fields.<name>.as(...)`
// values are kit's own code reading kit's own cache, so the page renders the
// submission without knowing one happened.
type Form struct {
	// ID is the client-side action id: `<hash>/<name>`.
	ID string `json:"id"`
	// Output is `{submission, result}` or `{submission, issues, input}` in
	// devalue's flat form, encoded with the app's transport so a `result`
	// carrying a custom type arrives as an instance of its class.
	Output string `json:"output"`
}

// Error is kit's `App.Error`: status and message, plus whatever extra
// properties the app's `handleError` hook added. Kit's own type has no fixed
// shape beyond the two fields every error carries — an app may augment
// `App.Error` with fields of its own — so this flattens to one JSON object
// either way, which is what lets a component read `$page.error.someField`
// without skgo committing to any field beyond the two kit's own type
// guarantees.
type Error struct {
	Status  int
	Message string
	// Extra is whatever else the hook returned: a merge target rather than a
	// fixed struct, because kit's own hook contract is "return only the
	// properties you want to override" against a shape the app itself defines.
	Extra map[string]any
}

// MarshalJSON flattens Extra alongside status and message, so a value with
// no extra properties round-trips exactly as the plain `{status, message}`
// object kit itself writes.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	out := make(map[string]any, len(e.Extra)+2)
	for k, v := range e.Extra {
		out[k] = v
	}
	if e.Status != 0 {
		out["status"] = e.Status
	}
	out["message"] = e.Message
	return json.Marshal(out)
}

// UnmarshalJSON reads status and message out by name and keeps everything
// else as Extra, the inverse of MarshalJSON.
func (e *Error) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["status"]; ok {
		if err := json.Unmarshal(v, &e.Status); err != nil {
			return err
		}
		delete(raw, "status")
	}
	if v, ok := raw["message"]; ok {
		if err := json.Unmarshal(v, &e.Message); err != nil {
			return err
		}
		delete(raw, "message")
	}
	if len(raw) == 0 {
		return nil
	}
	e.Extra = make(map[string]any, len(raw))
	for k, v := range raw {
		var value any
		if err := json.Unmarshal(v, &value); err != nil {
			return err
		}
		e.Extra[k] = value
	}
	return nil
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

// Engine is a pool of runtimes sharing one source of application code.
//
// In a build that source is one compiled program: the bundle the adapter
// emitted. Under `vite dev` there is no bundle — kit never runs an adapter
// there — and the source is the dev server itself, which the runtimes pull one
// transformed module at a time from. The engine is otherwise the same in both:
// the same entry, the same host bindings, the same pool, the same drain.
type Engine struct {
	program *goja.Program
	// dev is the running dev server the runtimes load their modules from, and
	// is nil for a build. entry is the module it is asked for first, and
	// prelude is the script every runtime evaluates before any module does —
	// the polyfill a build carries as its bundle's banner.
	dev     *vite.Dev
	entry   string
	prelude []byte
	// console is where every runtime's `console` reports.
	console Console
	// idle holds runtimes that are not rendering. A runtime is only ever
	// checked out by one goroutine, which is what keeps a render from
	// re-entering one.
	idle chan *runtime
	// permits bounds how many runtimes exist at once. Taking a permit is what
	// a caller waits on when every runtime is busy.
	permits   chan struct{}
	hostSlots chan struct{}

	mu      sync.Mutex
	created int
}

// runtime is one goja Runtime with the bundle evaluated in it.
type runtime struct {
	work   *renderWork
	vm     *goja.Runtime
	render goja.Callable
	// tick runs the next callback on the engine's macrotask queue and pending
	// says how many are waiting. See globalsSource: kit's `query.batch` flushes
	// with `setTimeout(..., 0)`, and this is the loop that lets it.
	tick    goja.Callable
	pending goja.Callable
	// reset empties that queue. A runtime is pooled, and work the last render
	// left behind is not this one's to run.
	reset goja.Callable

	// runner is the module runner this runtime's application code comes from
	// under `vite dev`, and is nil for a build. Each runtime has its own,
	// because each holds its own module instances and they are brought up to
	// date as they are checked out of the pool.
	runner *vite.Runner

	// host is the answer function of the render currently in flight. It is
	// written and read by the one goroutine holding the runtime.
	host Host
	// fetch and match are the other two calls a render currently in flight may
	// make back out to Go. Like host, they belong to the one goroutine holding
	// the runtime.
	fetch Fetch
	match Match
	// calls records the `<id>` and payload of every remote call the current
	// render made, in order.
	calls []Call
	// failed is set when a host call could not be answered, so that the
	// failure surfaces as a Go error rather than as a rendered error message.
	failed error
	// route is the route id of the render in flight, so that a line the engine
	// writes to `console` says which page wrote it. Like host, it is written
	// and read by the one goroutine holding the runtime.
	route string
}

// Call is one remote-function call a render made.
type Call struct {
	ID      string
	Payload string
}

// Fetch answers one render-time `event.fetch` of the app's own routes. The
// bytes in are a FetchRequest, JSON-encoded by the bundle's own polyfill; the
// bytes out are a FetchAnswer, JSON-encoded, which the bundle turns into a
// Response or throws as a TypeError. It is the only I/O a render's fetch ever
// performs: an in-process call into Go, never a socket the engine opens.
type Fetch func(ctx context.Context, request []byte) ([]byte, error)

// FetchRequest is one render-time `event.fetch` call, on its way into Go.
// Kit resolves a relative fetch against the page's own URL before this ever
// leaves the engine (`runtime/server/fetch.js`, `normalize_fetch_input`), so
// URL always carries an absolute, same-origin URL.
type FetchRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	// Headers is the request's headers, one value per name — the shape the
	// bundle's own `Headers` polyfill stores them in.
	Headers map[string]string `json:"headers,omitempty"`
	// Body is the request body as text, or "" for none. A render-time fetch
	// is for the app's own JSON and text endpoints; a binary body does not
	// cross this boundary.
	Body string `json:"body,omitempty"`
}

// FetchAnswer is what Go gave a render-time fetch: a response, or the message
// of the TypeError a real `fetch` throws when it cannot reach the target.
type FetchAnswer struct {
	Response *FetchResponse `json:"response,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// FetchResponse is one answer to a render-time fetch.
type FetchResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// Match answers one render-time `$app/paths` `match(pathname)` call. pathname
// is already decoded and stripped of the app's base, exactly as kit's own
// `match` prepares it before it asks the manifest (`runtime/app/paths/
// server.js`). Go answers from the same route table every page, load and
// endpoint request matches against, so a render's match is a call back into
// Go rather than a second router built for the engine — nil params means the
// route takes none, and ok false is kit's own answer for a pathname nothing
// matched: null.
type Match func(pathname string) (routeID string, params map[string]string, ok bool)

// Hosts is what Go answers a render's calls back out to it with. Each is
// optional; leaving one nil means a render that reaches it gets a well-defined
// refusal rather than a ReferenceError.
type Hosts struct {
	// Remote answers a remote-function call.
	Remote Host
	// Fetch answers a same-origin `event.fetch`.
	Fetch Fetch
	// Match answers a `$app/paths` `match()` lookup.
	Match Match
}

// New compiles the bundle and returns an engine that will create at most size
// runtimes. A size below one is one. console is told what the engine writes to
// `console`; a nil one logs.
func New(name string, source []byte, size int, console Console) (*Engine, error) {
	program, err := goja.Compile(name, string(source), false)
	if err != nil {
		return nil, fmt.Errorf("skgo: the SSR bundle does not parse: %w", err)
	}
	if size < 1 {
		size = 1
	}
	if console == nil {
		console = defaultConsole
	}
	return start(&Engine{program: program, console: console}, size)
}

// NewDev returns an engine whose application code is the running dev server's.
//
// `vite dev` never runs an adapter — kit reaches `adapt()` only from the
// plugin that finalises a build — so there is no bundle here. The adapter
// declares the same `goja` environment in the dev server instead, and each
// runtime pulls the entry and everything it imports out of it through vite's
// own `fetchModule`. prelude is the script a build carries as its bundle's
// banner, run before any module evaluates; entry is the module the engine
// comes up on.
func NewDev(dev *vite.Dev, entry string, prelude []byte, size int, console Console) (*Engine, error) {
	if size < 1 {
		size = 1
	}
	if console == nil {
		console = defaultConsole
	}
	return start(&Engine{dev: dev, entry: entry, prelude: prelude, console: console}, size)
}

// start fills in the pool and proves one runtime comes up.
func start(e *Engine, size int) (*Engine, error) {
	e.idle = make(chan *runtime, size)
	e.hostSlots = make(chan struct{}, maxHostCalls)
	e.permits = make(chan struct{}, size)
	for range size {
		e.permits <- struct{}{}
	}
	// One runtime is built now rather than on the first request, so that an
	// application the engine cannot evaluate is a startup failure instead of a
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

	// Before the bundle, not after: the bundle's own top-level code calls
	// console, and a bundle that cannot come up has to be able to say why.
	if err := rt.installGlobals(e.console); err != nil {
		return nil, err
	}

	// URL, the text codecs and base64, natively. Kit's runtime constructs a
	// URL and a TextEncoder while its modules are still evaluating, so these
	// have to be in place before the bundle runs at all.
	if err := rt.installWebGlobals(); err != nil {
		return nil, err
	}

	if err := rt.vm.Set("__skgo_remote", func(id, payload string) *goja.Promise {
		rt.calls = append(rt.calls, Call{ID: id, Payload: payload})
		host := rt.host
		return rt.promise(func(ctx context.Context) ([]byte, error) {
			if host == nil {
				return nil, fmt.Errorf("skgo: nothing is answering remote functions for this render (%s)", id)
			}
			return host(ctx, id, payload)
		})
	}); err != nil {
		return nil, err
	}
	if err := rt.vm.Set("__skgo_fetch", func(payload string) *goja.Promise {
		fetch := rt.fetch
		return rt.promise(func(ctx context.Context) ([]byte, error) {
			if fetch == nil {
				return nil, errors.New("skgo: nothing is answering event.fetch for this render")
			}
			return fetch(ctx, []byte(payload))
		})
	}); err != nil {
		return nil, err
	}

	if err := rt.vm.Set("__skgo_match", func(pathname string) (string, error) {
		if rt.match == nil {
			return "null", nil
		}
		routeID, params, ok := rt.match(pathname)
		if !ok {
			return "null", nil
		}
		raw, err := json.Marshal(struct {
			ID     string            `json:"id"`
			Params map[string]string `json:"params"`
		}{ID: routeID, Params: params})
		if err != nil {
			rt.failed = errors.Join(rt.failed, err)
			return "", err
		}
		return string(raw), nil
	}); err != nil {
		return nil, err
	}

	if e.dev != nil {
		// The polyfill is a build's bundle banner, and it has to keep that
		// position: Svelte's no-AsyncLocalStorage fallback is selected by a
		// flag it sets, and the first module that reads it would otherwise
		// throw `async_local_storage_unavailable`.
		if _, err := rt.vm.RunString(string(e.prelude)); err != nil {
			return nil, fmt.Errorf("skgo: installing the engine's polyfill: %w", err)
		}
		if _, err := rt.vm.RunString(devGlobalsSource); err != nil {
			return nil, fmt.Errorf("skgo: installing the dev engine's globals: %w", err)
		}
		rt.runner = vite.NewRunner(rt.vm, e.dev)
		if err := rt.runner.Boot(e.entry); err != nil {
			return nil, err
		}
	} else {
		if _, err := rt.vm.RunProgram(e.program); err != nil {
			return nil, fmt.Errorf("skgo: evaluating the SSR bundle: %w", err)
		}
		// Kit and Svelte each install their AsyncLocalStorage from a `.then()`
		// on a dynamic import, so neither is in place until the job queue has
		// been drained once. goja drains it when the outermost call returns;
		// this no-op run is that call. Without it the first async component
		// fails with kit's "Could not get the request store" in the middle of a
		// render. The dev path does the same, in Runner.Boot.
		if _, err := rt.vm.RunString("void 0"); err != nil {
			return nil, err
		}
	}

	ping, ok := goja.AssertFunction(rt.vm.Get("__skgo_ping"))
	if !ok {
		return nil, errors.New("skgo: the SSR bundle defines no __skgo_ping; it is not a bundle this build of skgo can run")
	}
	if v, err := ping(goja.Undefined()); err != nil || v.String() != "ok" {
		return nil, fmt.Errorf("skgo: the SSR bundle did not come up: %v", err)
	}

	if err := rt.bindRender(); err != nil {
		return nil, err
	}

	if rt.tick, ok = goja.AssertFunction(rt.vm.Get("__skgo_tick")); !ok {
		return nil, errors.New("skgo: the engine's globals define no __skgo_tick")
	}
	if rt.pending, ok = goja.AssertFunction(rt.vm.Get("__skgo_pending")); !ok {
		return nil, errors.New("skgo: the engine's globals define no __skgo_pending")
	}
	if rt.reset, ok = goja.AssertFunction(rt.vm.Get("__skgo_reset")); !ok {
		return nil, errors.New("skgo: the engine's globals define no __skgo_reset")
	}

	e.mu.Lock()
	e.created++
	e.mu.Unlock()
	return rt, nil
}

// Render renders one page, answering every call it makes back out to Go with
// hosts. routeID is kit's route id, and is only used to say which page a line
// the engine wrote to `console` came from. It returns the calls the render
// made, in order, so the caller can serialise exactly the answers it gave into
// the document.
func (e *Engine) Render(ctx context.Context, routeID string, request []byte, hosts Hosts) (Result, []Call, error) {
	rt, err := e.acquire(ctx)
	if err != nil {
		return Result{}, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	rt.work = &renderWork{ctx: ctx, slots: e.hostSlots, completed: make(chan hostCompletion, maxHostCalls)}
	reusable := false
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { rt.vm.Interrupt(ctx.Err()); close(interrupted) })
	defer func() {
		if !stop() {
			<-interrupted
		}
		clean := reusable && ctx.Err() == nil && rt.work.outstanding == 0
		cancel()
		rt.work = nil
		if clean {
			e.release(rt)
		} else {
			e.permits <- struct{}{}
		}
	}()

	// Under `vite dev` the application this runtime holds may be older than the
	// files on disk. Dropping exactly what an edit invalidated is what makes
	// the next document carry it.
	if err := e.refresh(rt); err != nil {
		return Result{}, nil, err
	}

	rt.host = hosts.Remote
	rt.fetch = hosts.Fetch
	rt.match = hosts.Match
	rt.calls = nil
	rt.failed = nil
	rt.route = routeID
	defer func() { rt.host = nil; rt.fetch = nil; rt.match = nil; rt.route = "" }()

	// Whatever the last render left on the macrotask queue belongs to a request
	// that is over. It is dropped rather than run: it would run against this
	// render's state, and the value it produced would be recorded against this
	// page.
	if _, err := rt.reset(goja.Undefined()); err != nil {
		return Result{}, nil, fmt.Errorf("skgo: clearing the runtime's pending work: %w", err)
	}

	v, err := rt.render(goja.Undefined(), rt.vm.ToValue(string(request)))
	if err != nil {
		return Result{}, rt.calls, fmt.Errorf("skgo: rendering: %w", err)
	}
	if err := rt.drain(v); err != nil {
		return Result{}, rt.calls, err
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
	reusable = true
	if result.Err != "" {
		return result, rt.calls, fmt.Errorf("skgo: the page threw while rendering: %s", result.Err)
	}
	return result, rt.calls, nil
}

// maxTicks bounds the macrotask queue so a render that keeps rescheduling
// itself fails instead of holding the runtime for ever. Nothing in kit's render
// path schedules more than one callback per batch query, so the bound is far
// above anything a real page reaches.
const maxTicks = 10_000

// drain runs the engine's macrotask queue until the render has finished or
// nothing is left to run.
//
// goja drains the promise jobs a call queued when that call returns, so a
// callback has to be entered from out here for its `await`s to make progress.
// That is also what makes this the right moment: a `setTimeout(fn, 0)` runs
// after the current turn's microtasks, and the current turn's microtasks are
// exactly what has just finished.
func (rt *runtime) drain(result goja.Value) error {
	object := result.ToObject(rt.vm)
	ticks := 0
	for !boolOf(object.Get("done")) {
		w := rt.work
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if rt.runner != nil {
			if err := rt.runner.Settle(); err != nil {
				return err
			}
			if boolOf(object.Get("done")) {
				return nil
			}
		}
		// Launch every runnable Go operation before waiting for any one.
		for len(w.queue) > 0 {
			select {
			case w.slots <- struct{}{}:
				w.start()
			default:
				goto launched
			}
		}
	launched:
		// Give completed I/O a turn even when JS keeps scheduling macrotasks.
		select {
		case c := <-w.completed:
			if err := rt.complete(c); err != nil {
				return err
			}
			continue
		default:
		}
		waiting, err := rt.pending(goja.Undefined())
		if err != nil {
			return fmt.Errorf("skgo: reading the render's pending work: %w", err)
		}
		if waiting.ToInteger() > 0 {
			if ticks == maxTicks {
				return fmt.Errorf("skgo: the render is still scheduling work after %d turns; it will not finish", maxTicks)
			}
			ticks++
			if _, err := rt.tick(goja.Undefined()); err != nil {
				return fmt.Errorf("skgo: running the render's pending work: %w", err)
			}
			continue
		}
		if w.outstanding == 0 {
			return nil
		}
		var available chan struct{}
		if len(w.queue) > 0 {
			available = w.slots
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case available <- struct{}{}:
			w.start()
		case c := <-w.completed:
			if err := rt.complete(c); err != nil {
				return err
			}
		}
	}
	return nil
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

// bindRender reads the entry's render function off the runtime. It is re-read
// after every hot update: re-importing the entry assigns a new closure over new
// module instances, and the old function would render the code that changed.
func (rt *runtime) bindRender() error {
	render, ok := goja.AssertFunction(rt.vm.Get("__skgo_render"))
	if !ok {
		return errors.New("skgo: the SSR bundle defines no __skgo_render")
	}
	rt.render = render
	return nil
}

// devGlobalsSource is what kit's own dev server puts on the global object and a
// build states as a define instead. `__SVELTEKIT_TRACK__` is kit's dev-only
// feature check (`exports/vite/dev/index.js`), which has nothing to report in
// an engine that is not kit's dev server; `__SVELTEKIT_PAYLOAD__` is the object
// kit's client reads its payload out of, and no module in the engine writes to
// it.
const devGlobalsSource = `
	globalThis.__SVELTEKIT_TRACK__ = function () {};
	globalThis.__SVELTEKIT_PAYLOAD__ = {};
	globalThis.process = globalThis.process || {};
	globalThis.process.env = globalThis.process.env || {};
`

// refresh brings a runtime up to date with the dev server before it renders.
// It is a no-op for a build, whose code cannot change while it is running.
func (e *Engine) refresh(rt *runtime) error {
	if rt.runner == nil {
		return nil
	}
	dropped, err := rt.runner.Refresh()
	if err != nil {
		return err
	}
	if dropped == 0 {
		return nil
	}
	if dropped < 0 {
		e.console("", "info", fmt.Sprintf("skgo: the dev server restarted; reloaded %d module(s)", rt.runner.Modules()))
	} else {
		e.console("", "info", fmt.Sprintf("skgo: reloaded %d of %d module(s) after an edit", dropped, rt.runner.Modules()))
	}
	return rt.bindRender()
}

// acquire takes an idle runtime, or builds one while the pool is below its
// size, or waits for one to come back.
func (e *Engine) acquire(ctx context.Context) (*runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case rt := <-e.idle:
		return rt, nil
	default:
	}
	select {
	case rt := <-e.idle:
		return rt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
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
