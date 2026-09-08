package vite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dop251/goja"
)

// server is a stand-in for `vite dev`: it answers the three endpoints the
// adapter's dev plugin adds, over modules a test writes by hand in the shape
// vite's SSR transform emits. What it proves is the contract the runner
// implements — the six identifiers, the result shape, and the change log — not
// vite itself, which has its own tests and is exercised end to end by the
// Gherkin suite.
type server struct {
	*httptest.Server

	mu      sync.Mutex
	modules map[string]Module
	changed []string
	// fetched counts how many times each module was asked for, which is how a
	// test tells a module that was reloaded from one that was kept.
	fetched map[string]int
}

func newServer(t *testing.T, modules map[string]Module) *server {
	t.Helper()
	s := &server{modules: modules, fetched: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/__skgo_dev/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Info{Entry: "/entry.js", GlobalName: "__sveltekit_dev",
			Client: Client{Start: "/start.js", App: "/app.js"}})
	})
	mux.HandleFunc("/__skgo_dev/module", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ URL, Importer string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		module, ok := s.modules[body.URL]
		s.fetched[body.URL]++
		s.mu.Unlock()
		if !ok {
			writeJSON(w, Module{Error: "no module " + body.URL})
			return
		}
		writeJSON(w, module)
	})
	mux.HandleFunc("/__skgo_dev/changed", func(w http.ResponseWriter, r *http.Request) {
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		s.mu.Lock()
		defer s.mu.Unlock()
		out := Changes{Version: len(s.changed)}
		if since > len(s.changed) {
			out.Reset = true
		} else {
			out.Files = append([]string(nil), s.changed[since:]...)
		}
		writeJSON(w, out)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// change edits a module the way an editor would, and tells the change log.
func (s *server) change(name string, module Module) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules[name] = module
	s.changed = append(s.changed, module.File)
}

func (s *server) fetchCount(url string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetched[url]
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// esm writes one module the way vite's SSR transform does: imports become
// `__vite_ssr_import__` calls, exports become `__vite_ssr_exportName__`.
func esm(file, code string) Module {
	return Module{Code: code, File: file, URL: file}
}

func boot(t *testing.T, s *server) (*goja.Runtime, *Runner) {
	t.Helper()
	vm := goja.New()
	runner := NewRunner(vm, NewDev(s.URL))
	if err := runner.Boot("/entry.js"); err != nil {
		t.Fatalf("booting: %v", err)
	}
	return vm, runner
}

// The whole contract in one graph: a static import, a re-export through
// `exportAll`, a getter export whose value is assigned after the import that
// reads it, and a dynamic import nobody awaits.
func TestTheRunnerEvaluatesAModuleGraph(t *testing.T) {
	s := newServer(t, map[string]Module{
		"/entry.js": esm("/entry.js", `
			const dep = await __vite_ssr_import__("/greeting.js", {"importedNames":["greeting"]});
			const late = await __vite_ssr_import__("/late.js", {"importedNames":["value"]});
			globalThis.__result = dep.greeting + " " + late.value;
			__vite_ssr_dynamic_import__("./aside.js");
		`),
		"/greeting.js": esm("/greeting.js", `
			const shared = await __vite_ssr_import__("/shared.js", {});
			__vite_ssr_exportAll__(shared);
			__vite_ssr_exportName__("greeting", () => "hello");
		`),
		"/shared.js": esm("/shared.js", `__vite_ssr_exportName__("shared", () => "yes");`),
		// The export is read through a getter, so a value assigned after the
		// importer already has the namespace is still visible.
		"/late.js": esm("/late.js", `
			let value = "unset";
			__vite_ssr_exportName__("value", () => value);
			value = "world";
		`),
		"/aside.js": esm("/aside.js", `globalThis.__aside = "settled";`),
	})

	vm, runner := boot(t, s)

	if got := vm.Get("__result").String(); got != "hello world" {
		t.Errorf("the entry computed %q, want %q", got, "hello world")
	}
	// A dynamic import at module scope that nobody awaits still has to settle:
	// kit and Svelte both install their AsyncLocalStorage that way, and a
	// runner that stopped at the entry's own promise would leave them pending.
	if got := vm.Get("__aside"); got == nil || got.String() != "settled" {
		t.Errorf("the un-awaited dynamic import never settled: %v", got)
	}
	if runner.Modules() != 5 {
		t.Errorf("the runner holds %d module(s), want 5", runner.Modules())
	}
}

// Two modules that import each other. The one still evaluating hands back the
// exports object it is filling in, which is the live binding a cycle needs.
func TestTheRunnerResolvesAnImportCycle(t *testing.T) {
	s := newServer(t, map[string]Module{
		"/entry.js": esm("/entry.js", `
			const a = await __vite_ssr_import__("/a.js", {"importedNames":["a"]});
			globalThis.__result = a.a();
		`),
		"/a.js": esm("/a.js", `
			const b = await __vite_ssr_import__("/b.js", {"importedNames":["b"]});
			__vite_ssr_exportName__("a", () => () => "a then " + b.b());
		`),
		"/b.js": esm("/b.js", `
			await __vite_ssr_import__("/a.js", {"importedNames":["a"]});
			__vite_ssr_exportName__("b", () => () => "b");
		`),
	})

	vm, _ := boot(t, s)
	if got := vm.Get("__result").String(); got != "a then b" {
		t.Errorf("the cycle produced %q, want %q", got, "a then b")
	}
}

// An edit drops the module it changed and everything that imported it, and
// nothing else. The counts are the claim: a module nobody's edit reached is
// still the instance the last render used.
func TestAnEditDropsOnlyWhatItReached(t *testing.T) {
	s := newServer(t, map[string]Module{
		"/entry.js": esm("/entry.js", `
			const page = await __vite_ssr_import__("/page.js", {"importedNames":["title"]});
			await __vite_ssr_import__("/untouched.js", {});
			globalThis.__title = page.title;
		`),
		"/page.js":      esm("/page.js", `__vite_ssr_exportName__("title", () => "Home");`),
		"/untouched.js": esm("/untouched.js", `globalThis.__untouched = (globalThis.__untouched ?? 0) + 1;`),
	})

	vm, runner := boot(t, s)
	if got := vm.Get("__title").String(); got != "Home" {
		t.Fatalf("the first render saw %q, want %q", got, "Home")
	}

	s.change("/page.js", esm("/page.js", `__vite_ssr_exportName__("title", () => "Home, edited");`))

	dropped, err := runner.Refresh()
	if err != nil {
		t.Fatalf("refreshing: %v", err)
	}
	// The edited module and the entry that imported it, and not the third.
	if dropped != 2 {
		t.Errorf("the edit dropped %d module(s), want 2", dropped)
	}
	if got := vm.Get("__title").String(); got != "Home, edited" {
		t.Errorf("after the edit the entry saw %q, want %q", got, "Home, edited")
	}
	if got := s.fetchCount("/untouched.js"); got != 1 {
		t.Errorf("the untouched module was fetched %d time(s), want 1", got)
	}
	if got := vm.Get("__untouched").ToInteger(); got != 1 {
		t.Errorf("the untouched module was evaluated %d time(s), want 1", got)
	}
}

// A module with no file — the node table is generated rather than read — is
// named in the change log by its URL, and that has to drop it too.
func TestAModuleWithNoFileIsDroppedByItsURL(t *testing.T) {
	const table = "/@id/__x00__skgo:skgo:nodes"
	s := newServer(t, map[string]Module{
		"/entry.js": esm("/entry.js", fmt.Sprintf(`
			const nodes = await __vite_ssr_import__(%q, {"importedNames":["components"]});
			globalThis.__components = nodes.components;
		`, table)),
		table: {Code: `__vite_ssr_exportName__("components", () => "one");`, URL: table},
	})

	vm, runner := boot(t, s)
	if got := vm.Get("__components").String(); got != "one" {
		t.Fatalf("the first render saw %q, want %q", got, "one")
	}

	s.mu.Lock()
	s.modules[table] = Module{Code: `__vite_ssr_exportName__("components", () => "two");`, URL: table}
	s.changed = append(s.changed, table)
	s.mu.Unlock()

	if _, err := runner.Refresh(); err != nil {
		t.Fatalf("refreshing: %v", err)
	}
	if got := vm.Get("__components").String(); got != "two" {
		t.Errorf("after the node table changed the entry saw %q, want %q", got, "two")
	}
}

// A dev server that restarted answers a cursor it has never issued. Nothing in
// the cache can be trusted against a module graph this runtime never saw.
func TestARestartedDevServerReloadsEverything(t *testing.T) {
	s := newServer(t, map[string]Module{
		"/entry.js": esm("/entry.js", `globalThis.__boots = (globalThis.__boots ?? 0) + 1;`),
	})
	s.change("/entry.js", esm("/entry.js", `globalThis.__boots = (globalThis.__boots ?? 0) + 1;`))

	vm, runner := boot(t, s)
	if got := vm.Get("__boots").ToInteger(); got != 1 {
		t.Fatalf("the entry evaluated %d time(s), want 1", got)
	}

	// The change log goes back to empty, which is a cursor from the future.
	s.mu.Lock()
	s.changed = nil
	s.mu.Unlock()

	dropped, err := runner.Refresh()
	if err != nil {
		t.Fatalf("refreshing: %v", err)
	}
	if dropped != -1 {
		t.Errorf("a restart reported %d dropped, want -1", dropped)
	}
	if got := vm.Get("__boots").ToInteger(); got != 2 {
		t.Errorf("the entry evaluated %d time(s) after the restart, want 2", got)
	}
}

// A module the dev server cannot transform is a Go error naming the module,
// not a silent empty namespace.
func TestATransformFailureNamesTheModule(t *testing.T) {
	s := newServer(t, map[string]Module{
		"/entry.js":  esm("/entry.js", `await __vite_ssr_import__("/broken.js", {});`),
		"/broken.js": {Error: "Unexpected token"},
	})

	vm := goja.New()
	runner := NewRunner(vm, NewDev(s.URL))
	err := runner.Boot("/entry.js")
	if err == nil {
		t.Fatal("booting a graph with a broken module succeeded")
	}
	if !strings.Contains(err.Error(), "/broken.js") || !strings.Contains(err.Error(), "Unexpected token") {
		t.Errorf("the failure was reported as %q, which names neither the module nor the reason", err)
	}
}

// The engine has no module loader, so a module vite decided to externalize is a
// failure that says so rather than a namespace with nothing in it.
func TestAnExternalizedModuleIsRefused(t *testing.T) {
	s := newServer(t, map[string]Module{
		"/entry.js": esm("/entry.js", `await __vite_ssr_import__("node:fs", {});`),
		"node:fs":   {Externalize: "node:fs", Type: "builtin"},
	})

	vm := goja.New()
	runner := NewRunner(vm, NewDev(s.URL))
	err := runner.Boot("/entry.js")
	if err == nil {
		t.Fatal("booting a graph with an externalized module succeeded")
	}
	if !strings.Contains(err.Error(), "node:fs") {
		t.Errorf("the failure was reported as %q, which does not name the module", err)
	}
}
