// Package ssrspike is a feasibility spike, not production code. It asks one
// question: can a JS engine embedded in the Go process server-render the
// example app's own compiled Svelte components, with every remote function
// answered by Go in-process and no Node anywhere at request time?
//
// The bundle it loads is built by ephemeral/spike-ssr/build.mjs from the
// example app's real sources and kit's real runtime. Nothing here reimplements
// kit: the JavaScript that runs is `render(Root, { props, context })` from
// svelte/server with kit's own `Props`/`RenderNode`, exactly as
// packages/kit/src/runtime/server/page/render.js:142-261 builds it.
package ssrspike

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/dop251/goja"
)

// Answer is what Go hands back for one remote-function call. It is the shape
// kit serialises into `__sveltekit.data`: a value, or an error.
type Answer struct {
	Value json.RawMessage `json:"v,omitempty"`
	Error *AnswerError    `json:"e,omitempty"`
}

// AnswerError mirrors kit's `App.Error`.
type AnswerError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// RemoteFunc answers a remote-function call during a render. `id` is
// `<hash>/<name>` as kit's vite plugin assigns it
// (packages/kit/src/exports/vite/index.js:678) and `payload` is
// `stringify_remote_arg(arg)`. Together they are `create_remote_key(id,
// payload)` — the key the browser's query cache will look the value up under.
type RemoteFunc func(id, payload string) Answer

// Result is one render.
type Result struct {
	Done  bool   `json:"done"`
	Error string `json:"error"`
	Head  string `json:"head"`
	Body  string `json:"body"`
	// Data is `state.remote.implicit` collected and settled, keyed by
	// `<remote id>/<payload>` under the single-letter type kit uses
	// ("q" for query, "l" for live query, "f" for form, "p" for prerender).
	Data map[string]map[string]Answer `json:"data"`
}

// Engine is one goja Runtime with the SSR bundle evaluated in it. It is not
// safe for concurrent use: goja Runtimes never are, and neither is the
// AsyncLocalStorage shim the bundle relies on.
type Engine struct {
	vm     *goja.Runtime
	render goja.Callable

	mu     sync.Mutex
	remote RemoteFunc
	calls  []string
}

// Program compiles the bundle once so that many Engines can share the parse.
func Program(path string) (*goja.Program, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return goja.Compile(path, string(src), false)
}

// New evaluates prog in a fresh Runtime.
func New(prog *goja.Program) (*Engine, error) {
	e := &Engine{vm: goja.New()}

	if err := e.vm.Set("__skgo_remote", func(id, payload string) (string, error) {
		e.mu.Lock()
		fn := e.remote
		e.calls = append(e.calls, id+"/"+payload)
		e.mu.Unlock()
		if fn == nil {
			return "", fmt.Errorf("skgo: no remote handler installed for %s/%s", id, payload)
		}
		out, err := json.Marshal(fn(id, payload))
		if err != nil {
			return "", err
		}
		return string(out), nil
	}); err != nil {
		return nil, err
	}

	if _, err := e.vm.RunProgram(prog); err != nil {
		return nil, fmt.Errorf("evaluating bundle: %w", err)
	}

	// Both kit and svelte install their AsyncLocalStorage from a `.then()` on a
	// dynamic import (kit/src/exports/internal/server/event.js:17,
	// svelte/src/internal/server/render-context.js:73), so it is not in place
	// until the promise job queue has been drained once. goja drains it when the
	// outermost call returns, so this no-op run is what puts `als` in place.
	if _, err := e.vm.RunString("void 0"); err != nil {
		return nil, err
	}

	fn, ok := goja.AssertFunction(e.vm.Get("__skgo_render"))
	if !ok {
		return nil, fmt.Errorf("bundle did not define __skgo_render")
	}
	e.render = fn
	return e, nil
}

// Render renders one page. `request` is the JSON the bundle's entry expects:
// the parsed URL, the route id, the params, the status, the error, and the
// branch of node indices with the per-node data a Go load produced.
func (e *Engine) Render(request []byte, remote RemoteFunc) (Result, error) {
	e.mu.Lock()
	e.remote = remote
	e.calls = nil
	e.mu.Unlock()

	v, err := e.render(goja.Undefined(), e.vm.ToValue(string(request)))
	if err != nil {
		return Result{}, err
	}

	var r Result
	raw, err := json.Marshal(v.Export())
	if err != nil {
		return Result{}, err
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf("decoding render result: %w (%s)", err, raw)
	}
	return r, nil
}

// LeakedStore reports whether kit's request store is still populated after a
// render finished — the shim's known cost.
func (e *Engine) LeakedStore() (string, error) {
	v, err := e.vm.RunString("__skgo_leaked_store()")
	if err != nil {
		return "", err
	}
	return v.String(), nil
}

// Calls returns the `<id>/<payload>` keys the last render asked Go for, in
// order. It is how the test proves the engine really came back out to Go rather
// than finding something cached.
func (e *Engine) Calls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.calls))
	copy(out, e.calls)
	return out
}
