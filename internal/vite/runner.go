package vite

import (
	"fmt"
	"path"
	"strings"

	"github.com/dop251/goja"
)

// Runner evaluates the modules a dev server transforms, in one goja runtime.
//
// It is vite's module-runner contract and nothing more: the body of a
// transformed module is wrapped in an async function taking six names
// (`vite/src/module-runner/constants.ts`), and the runner supplies them. The
// event loop the contract needs is Go's — goja drains its job queue when the
// outermost call returns, which is what lets a module suspended on an import
// continue once Go has evaluated its dependency at the top level.
type Runner struct {
	vm  *goja.Runtime
	dev *Dev

	// entry is the module the engine is brought up by. It is re-imported after
	// every hot update, because dropping a module drops everything that
	// imported it and the entry is the root of all of them.
	entry string

	mods map[string]*module
	// importers is the reverse of the module graph: who has to be evaluated
	// again when a module changes. Vite's own runner keeps the same edge, and
	// it is the whole difference between a hot update and a rebuild.
	importers map[string][]string
	pending   []*request

	// cursor is how far through the dev server's change log this runtime has
	// been brought. Each runtime carries its own, because each has its own
	// module cache and they are updated as they are checked out of the pool.
	cursor int
}

type module struct {
	exports *goja.Object
	url     string
	file    string
}

// request is one `__vite_ssr_import__` a suspended module is waiting on. The
// promise is resolved from Go, at the top level, once the dependency has been
// evaluated.
type request struct {
	url      string
	importer string
	resolve  func(any) error
	reject   func(any) error
}

// NewRunner makes a runner for vm against dev. Nothing is fetched until Boot.
func NewRunner(vm *goja.Runtime, dev *Dev) *Runner {
	return &Runner{
		vm:        vm,
		dev:       dev,
		mods:      map[string]*module{},
		importers: map[string][]string{},
	}
}

// Boot evaluates the render entry and everything it reaches, and brings the
// runner's cursor up to whatever the dev server has already seen change. The
// runtime's globals must already be installed: kit's and Svelte's modules read
// them while they are still evaluating.
func (r *Runner) Boot(entry string) error {
	r.entry = entry
	changes, err := r.dev.Changed(0)
	if err != nil {
		return err
	}
	r.cursor = changes.Version
	if err := r.load(); err != nil {
		return err
	}
	return nil
}

// Refresh brings this runtime up to date with the dev server and reports how
// many modules it had to drop. Everything the changed files did not reach is
// still the instance the last render used.
func (r *Runner) Refresh() (int, error) {
	changes, err := r.dev.Changed(r.cursor)
	if err != nil {
		return 0, err
	}
	r.cursor = changes.Version
	if changes.Reset {
		// A dev server that restarted under us. Nothing in the cache can be
		// trusted against a module graph this runtime never saw.
		r.mods = map[string]*module{}
		r.importers = map[string][]string{}
		if err := r.load(); err != nil {
			return 0, err
		}
		return -1, nil
	}
	if len(changes.Files) == 0 {
		return 0, nil
	}
	dropped := r.invalidate(changes.Files)
	if dropped == 0 {
		return 0, nil
	}
	if err := r.load(); err != nil {
		return dropped, err
	}
	return dropped, nil
}

// load imports the entry and settles everything the import left pending.
func (r *Runner) load() error {
	if _, err := r.importModule(r.entry, ""); err != nil {
		return err
	}
	// Kit and Svelte both install their AsyncLocalStorage from a `.then()` on a
	// dynamic import at module scope, and nothing awaits either. A runner that
	// stopped as soon as the entry's own promise settled would leave them
	// pending for ever, and the first render would fail with kit's "Could not
	// get the request store".
	return r.Settle()
}

// Settle answers every import request nobody is waiting on and lets the
// continuations run. The engine's own drain loop calls it between turns: a
// dynamic import made during a render is one of these.
func (r *Runner) Settle() error {
	for len(r.pending) > 0 {
		req := r.pending[0]
		r.pending = r.pending[1:]
		exports, err := r.importModule(req.url, req.importer)
		if err != nil {
			if err := req.reject(r.vm.NewGoError(err)); err != nil {
				return err
			}
			continue
		}
		if err := req.resolve(exports); err != nil {
			return err
		}
	}
	// goja runs the jobs a call queued when that call returns; this no-op run
	// is that call.
	_, err := r.vm.RunString("void 0")
	return err
}

// Modules is how many modules this runtime holds. It is what a log line says
// after a cold boot and after a hot update.
func (r *Runner) Modules() int { return len(r.mods) }

// importModule evaluates one module and everything it imports. It is only ever
// called with no JavaScript on the stack.
func (r *Runner) importModule(url, importer string) (*goja.Object, error) {
	if m, ok := r.mods[url]; ok {
		// A module still being evaluated hands back the exports object it is
		// filling in — the live binding an import cycle needs.
		return m.exports, nil
	}

	res, err := r.dev.Module(url, importer)
	if err != nil {
		return nil, err
	}

	exportsValue, err := r.vm.RunString("Object.create(null)")
	if err != nil {
		return nil, err
	}
	exports := exportsValue.ToObject(r.vm)
	r.mods[url] = &module{exports: exports, url: url, file: res.File}

	fn, err := r.compile(url, res.Code)
	if err != nil {
		delete(r.mods, url)
		return nil, err
	}

	promiseValue, err := fn(goja.Undefined(), exports, r.meta(url, res.File),
		r.importFn(url), r.dynamicImportFn(url), r.exportAll(exports), r.exportName(exports))
	if err != nil {
		delete(r.mods, url)
		return nil, fmt.Errorf("skgo: evaluating %s: %w", url, err)
	}
	promise, ok := promiseValue.Export().(*goja.Promise)
	if !ok {
		return exports, nil
	}

	// The loop. Every `__vite_ssr_import__` the module made is a pending
	// promise; Go loads the dependency here, with the stack empty, and resolves
	// it, which lets goja run the continuation when this call returns.
	for promise.State() == goja.PromiseStatePending {
		if len(r.pending) == 0 {
			return nil, fmt.Errorf("skgo: %s never settled and is waiting on nothing", url)
		}
		req := r.pending[0]
		r.pending = r.pending[1:]
		dep, err := r.importModule(req.url, req.importer)
		if err != nil {
			if err := req.reject(r.vm.NewGoError(err)); err != nil {
				return nil, err
			}
			continue
		}
		if err := req.resolve(dep); err != nil {
			return nil, err
		}
	}
	if promise.State() == goja.PromiseStateRejected {
		delete(r.mods, url)
		return nil, fmt.Errorf("skgo: evaluating %s: %s", url, describe(r.vm, promise.Result()))
	}
	return exports, nil
}

// compile wraps a transformed module the way vite's own evaluator does
// (`module-runner/esmEvaluator.ts`): an async function of the six names the SSR
// transform emits calls to.
func (r *Runner) compile(url, code string) (goja.Callable, error) {
	wrapper := "(async function (__vite_ssr_exports__, __vite_ssr_import_meta__, __vite_ssr_import__," +
		" __vite_ssr_dynamic_import__, __vite_ssr_exportAll__, __vite_ssr_exportName__) {\n\"use strict\";\n" +
		code + "\n})"
	value, err := r.vm.RunScript(url, wrapper)
	if err != nil {
		return nil, fmt.Errorf("skgo: compiling %s: %w", url, err)
	}
	fn, ok := goja.AssertFunction(value)
	if !ok {
		return nil, fmt.Errorf("skgo: %s did not compile to a function", url)
	}
	return fn, nil
}

// meta is `import.meta` for one module.
func (r *Runner) meta(url, file string) *goja.Object {
	if file == "" {
		file = url
	}
	meta := r.vm.NewObject()
	_ = meta.Set("url", "file://"+file)
	_ = meta.Set("filename", file)
	_ = meta.Set("dirname", path.Dir(file))
	// `import.meta.env` is vite's, not kit's: DEV here describes the engine,
	// which never runs vite's client. What decides whether Svelte's components
	// are the dev build is `esm-env`, which the adapter's plugin substitutes.
	env := r.vm.NewObject()
	_ = env.Set("DEV", false)
	_ = env.Set("PROD", true)
	_ = env.Set("SSR", true)
	_ = env.Set("MODE", "development")
	_ = env.Set("BASE_URL", "/")
	_ = meta.Set("env", env)
	// `import.meta.hot` is undefined by design: HMR belongs to the browser,
	// which gets it from vite's own client. A module that tests for it here
	// takes its production path, which is the one the engine can run.
	return meta
}

func (r *Runner) importFn(url string) goja.Value {
	return r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.enqueue(call.Argument(0).String(), url)
	})
}

func (r *Runner) dynamicImportFn(url string) goja.Value {
	return r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		dep := call.Argument(0).String()
		// A static import has already been rewritten to a resolved URL by
		// vite's import analysis; a dynamic one may still be relative to the
		// module that made it.
		if strings.HasPrefix(dep, ".") {
			dep = path.Join(path.Dir(url), dep)
		}
		return r.enqueue(dep, url)
	})
}

func (r *Runner) enqueue(dep, importer string) goja.Value {
	r.importers[dep] = appendOnce(r.importers[dep], importer)
	promise, resolve, reject := r.vm.NewPromise()
	r.pending = append(r.pending, &request{url: dep, importer: importer, resolve: resolve, reject: reject})
	return r.vm.ToValue(promise)
}

// invalidate drops every module the dev server named, and everything that
// imported one, transitively. The next import re-fetches exactly those and
// leaves the rest of the graph in place.
//
// A name is a file for a module compiled from one, and a URL for a module that
// has no file: the node table is generated rather than read, so nothing else
// could attribute a change to it.
func (r *Runner) invalidate(names []string) int {
	changed := map[string]bool{}
	for _, name := range names {
		changed[name] = true
	}

	var queue []string
	for url, m := range r.mods {
		if changed[url] || (m.file != "" && changed[m.file]) {
			queue = append(queue, url)
		}
	}

	seen := map[string]bool{}
	for len(queue) > 0 {
		url := queue[0]
		queue = queue[1:]
		if seen[url] {
			continue
		}
		seen[url] = true
		delete(r.mods, url)
		queue = append(queue, r.importers[url]...)
	}
	return len(seen)
}

func appendOnce(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func (r *Runner) exportAll(exports *goja.Object) goja.Value {
	return r.bind(exportAllSource, exports)
}

func (r *Runner) exportName(exports *goja.Object) goja.Value {
	return r.bind(exportNameSource, exports)
}

// bind evaluates one of the two helpers below and closes it over this module's
// exports object.
func (r *Runner) bind(source string, exports *goja.Object) goja.Value {
	value, err := r.vm.RunString(source)
	if err != nil {
		// A constant in this file: unreachable unless it was edited wrong.
		panic("skgo: compiling the module runner's helpers: " + err.Error())
	}
	fn, ok := goja.AssertFunction(value)
	if !ok {
		panic("skgo: the module runner's helper did not compile to a function")
	}
	bound, err := fn(goja.Undefined(), exports)
	if err != nil {
		panic("skgo: binding the module runner's helper: " + err.Error())
	}
	return bound
}

// exportAllSource is `__vite_ssr_exportAll__`: `export * from '...'`, as live
// getters so a re-exported binding is read through rather than copied.
const exportAllSource = `(function (exports) {
	return function (sourceModule) {
		if (exports === sourceModule) return;
		if (sourceModule === null || typeof sourceModule !== 'object') return;
		for (const key in sourceModule) {
			if (key !== 'default' && key !== '__esModule' && !(key in exports)) {
				try {
					Object.defineProperty(exports, key, {
						enumerable: true,
						configurable: true,
						get: () => sourceModule[key]
					});
				} catch (e) {}
			}
		}
	};
})`

// exportNameSource is `__vite_ssr_exportName__`: one named export, as a getter,
// which is what makes a binding assigned later in the module body visible to
// whoever imported it earlier.
const exportNameSource = `(function (exports) {
	return function (name, getter) {
		Object.defineProperty(exports, name, {
			enumerable: true,
			configurable: true,
			get: getter
		});
	};
})`

// describe reads whatever a rejected module promise carried. An Error's stack
// says which line of which module threw; anything else is stringified.
func describe(vm *goja.Runtime, value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return "undefined"
	}
	if object := value.ToObject(vm); object != nil {
		if stack := object.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
			return stack.String()
		}
	}
	return value.String()
}
