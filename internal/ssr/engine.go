//go:build !skgo_development

package ssr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/dop251/goja"
)

// Engine is a pool of runtimes evaluating the adapter's compiled SSR bundle.
type Engine struct {
	program     *goja.Program
	environment []byte
	// console is where every runtime's `console` reports.
	console Console
	// pools[d] serves renders nested d levels deep. A runtime is only ever
	// checked out by one goroutine, which is what keeps a render from
	// re-entering one. A render that asks Go for another page's document holds
	// its own runtime while it waits, so the nested render needs one that is
	// not in the same pool: with every runtime of one pool held by a render
	// that is waiting for another, nothing could ever finish. Each level is
	// bounded by the same size, and the number of levels by MaxRenderDepth.
	size      int
	pools     []*pool
	hostSlots chan struct{}

	mu      sync.Mutex
	created int
}

// pool is one level's runtimes.
type pool struct {
	// idle holds runtimes that are not rendering.
	idle chan *runtime
	// permits bounds how many runtimes exist at once. Taking a permit is what
	// a caller waits on when every runtime is busy.
	permits chan struct{}
}

func newPool(size int) *pool {
	p := &pool{idle: make(chan *runtime, size), permits: make(chan struct{}, size)}
	for range size {
		p.permits <- struct{}{}
	}
	return p
}

func (e *Engine) pool(depth int) *pool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for len(e.pools) <= depth {
		e.pools = append(e.pools, newPool(e.size))
	}
	return e.pools[depth]
}

// runtime is one goja Runtime with the bundle evaluated in it.
type runtime struct {
	pool   *pool
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

	// host is the answer function of the render currently in flight. It is
	// written and read by the one goroutine holding the runtime.
	host Host
	// fetch and match are the other two calls a render currently in flight may
	// make back out to Go. Like host, they belong to the one goroutine holding
	// the runtime.
	fetch         Fetch
	match         Match
	clientAddress func() (string, error)
	tracing       func(context.Context, []byte) ([]byte, error)
	traceContext  func(context.Context, string) context.Context
	handleError   func(context.Context, []byte) ([]byte, error)
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

// New compiles the bundle and returns an engine that will create at most size
// runtimes. A size below one is one. console is told what the engine writes to
// `console`; a nil one logs.
func New(name string, source []byte, size int, console Console, environment ...[]byte) (*Engine, error) {
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
	return start(&Engine{program: program, console: console, environment: firstEnvironment(environment)}, size)
}

func firstEnvironment(values [][]byte) []byte {
	if len(values) == 0 {
		return nil
	}
	return append([]byte(nil), values[0]...)
}

// start fills in the pool and proves one runtime comes up.
func start(e *Engine, size int) (*Engine, error) {
	e.size = size
	e.pools = []*pool{newPool(size)}
	e.hostSlots = make(chan struct{}, maxHostCalls)
	// One runtime is built now rather than on the first request, so that an
	// application the engine cannot evaluate is a startup failure instead of a
	// broken page.
	rt, err := e.newRuntime()
	if err != nil {
		return nil, err
	}
	rt.pool = e.pools[0]
	<-rt.pool.permits
	rt.pool.idle <- rt
	return e, nil
}

// Size is how many runtimes the engine may hold.
func (e *Engine) Size() int { return e.size }

// Created is how many runtimes the engine has actually built. It is what a test
// reads to see that concurrent renders did not share one.
func (e *Engine) Created() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.created
}

func (e *Engine) newRuntime() (*runtime, error) {
	rt := &runtime{vm: goja.New()}
	if len(e.environment) > 0 {
		// Parse into a fresh JS object for every runtime; never share mutable
		// Go maps between pooled JavaScript engines.
		if _, err := rt.vm.RunString("globalThis.__skgo_environment = JSON.parse(" + strconv.Quote(string(e.environment)) + ");"); err != nil {
			return nil, fmt.Errorf("skgo: initializing renderer environment: %w", err)
		}
	}

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

	if err := rt.vm.Set("__skgo_remote", func(id, payload, tracing string) *goja.Promise {
		rt.calls = append(rt.calls, Call{ID: id, Payload: payload})
		host := rt.host
		operationCtx := rt.operationContext(tracing)
		return rt.promise(func(_ context.Context) ([]byte, error) {
			if host == nil {
				return nil, fmt.Errorf("skgo: nothing is answering remote functions for this render (%s)", id)
			}
			return host(operationCtx, id, payload)
		})
	}); err != nil {
		return nil, err
	}
	if err := rt.vm.Set("__skgo_fetch", func(payload, tracing string) *goja.Promise {
		fetch := rt.fetch
		operationCtx := rt.operationContext(tracing)
		return rt.promise(func(_ context.Context) ([]byte, error) {
			if fetch == nil {
				return nil, errors.New("skgo: nothing is answering event.fetch for this render")
			}
			return fetch(operationCtx, []byte(payload))
		})
	}); err != nil {
		return nil, err
	}

	if err := rt.vm.Set("__skgo_trace", func(payload string) (string, error) {
		if rt.work == nil || rt.tracing == nil {
			return `{"enabled":false}`, nil
		}
		raw, err := rt.tracing(rt.work.ctx, []byte(payload))
		return string(raw), err
	}); err != nil {
		return nil, err
	}
	if err := rt.vm.Set("__skgo_handle_error", func(payload string) (string, error) {
		if rt.handleError == nil {
			var input struct {
				Body json.RawMessage `json:"body"`
			}
			if err := json.Unmarshal([]byte(payload), &input); err != nil {
				return "", err
			}
			return string(input.Body), nil
		}
		body, err := rt.handleError(rt.work.ctx, []byte(payload))
		return string(body), err
	}); err != nil {
		return nil, err
	}
	if err := rt.vm.Set("__skgo_client_address", func() (string, error) {
		if rt.clientAddress == nil {
			return "", errors.New("skgo: client address is unavailable in this request context")
		}
		return rt.clientAddress()
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

	if _, err := rt.vm.RunProgram(e.program); err != nil {
		return nil, fmt.Errorf("skgo: evaluating the SSR bundle: %w", err)
	}
	// Drain dynamic-import jobs before the first render.
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
	depth := Depth(ctx)
	if depth >= MaxRenderDepth {
		return Result{}, nil, fmt.Errorf("skgo: %d renders are waiting on one another; the pages fetch each other in a cycle", depth)
	}
	rt, err := e.acquire(ctx, e.pool(depth))
	if err != nil {
		return Result{}, nil, err
	}
	// Everything this render asks Go for runs one level deeper.
	ctx, cancel := context.WithCancel(context.WithValue(ctx, depthKey{}, depth+1))
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
			rt.pool.idle <- rt
		} else {
			rt.pool.permits <- struct{}{}
		}
	}()

	rt.tracing, rt.traceContext = hosts.Tracing, hosts.TraceContext
	rt.host = hosts.Remote
	rt.fetch = hosts.Fetch
	rt.match = hosts.Match
	rt.clientAddress = hosts.ClientAddress
	rt.handleError = hosts.HandleError
	rt.calls = nil
	rt.failed = nil
	rt.route = routeID
	defer func() {
		rt.host = nil
		rt.fetch = nil
		rt.match = nil
		rt.clientAddress = nil
		rt.handleError = nil
		rt.tracing = nil
		rt.traceContext = nil
		rt.route = ""
	}()

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
		Done:    boolOf(object.Get("done")),
		Err:     stringOf(object.Get("failure")),
		Status:  intOf(object.Get("status")),
		Head:    stringOf(object.Get("head")),
		Body:    stringOf(object.Get("body")),
		Fetched: stringOf(object.Get("fetched")),
	}
	if err := decodeInto(rt.vm, object.Get("handled_error"), &result.HandledError); err != nil {
		return result, rt.calls, err
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
	if result.HandledError != nil {
		return result, rt.calls, &HandledError{Body: result.HandledError}
	}
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

// acquire takes an idle runtime, or builds one while the pool is below its
// size, or waits for one to come back.
func (e *Engine) acquire(ctx context.Context, p *pool) (*runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case rt := <-p.idle:
		return rt, nil
	default:
	}
	select {
	case rt := <-p.idle:
		return rt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.permits:
		rt, err := e.newRuntime()
		if err != nil {
			p.permits <- struct{}{}
			return nil, err
		}
		rt.pool = p
		return rt, nil
	}
}

func (rt *runtime) operationContext(id string) context.Context {
	if rt.work == nil {
		return context.Background()
	}
	if rt.traceContext == nil || id == "" {
		return rt.work.ctx
	}
	return rt.traceContext(rt.work.ctx, id)
}
