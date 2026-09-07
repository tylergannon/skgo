package ssrspike

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAsyncLocalStorageVariants renders the same page from three bundles that
// differ only in how the missing `node:async_hooks` is handled, and says which
// ones work.
//
//   - "none":         `import('node:async_hooks')` rejects, which is what a bare
//     engine gives you. svelte then throws `async_local_storage_unavailable`
//     (svelte/src/internal/server/render-context.js:49).
//   - "webcontainer": svelte's own supported fallback — a module-global context
//     plus a serialised render queue, chosen by
//     `globalThis.process.versions.webcontainer` (render-context.js:39,83). Kit
//     reads the same flag and stops nulling its synchronous request store
//     (kit/src/constants.js:30, event.js:82).
//   - "stub":         a hand-written AsyncLocalStorage whose store is per
//     instance and is never restored.
func TestAsyncLocalStorageVariants(t *testing.T) {
	dir := spikeDir(t)
	_, f := load(t)

	want := `<p data-testid="site-name">Anvil and Ampersand</p>`

	for _, v := range []struct{ name, file string }{
		{"none", "ssr.none.js"},
		{"webcontainer", "ssr.webcontainer.js"},
		{"stub", "ssr.js"},
	} {
		path := filepath.Join(dir, "dist", v.file)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("variant %q was never built (%s). Build all three:\n"+
				"\tcd ephemeral/spike-ssr && node build.mjs --als=none --out=dist/ssr.none.js "+
				"&& node build.mjs --als=webcontainer --out=dist/ssr.webcontainer.js && node build.mjs",
				v.name, path)
		}

		prog, err := Program(path)
		if err != nil {
			t.Errorf("als=%s: goja could not compile: %v", v.name, err)
			continue
		}
		e, err := New(prog)
		if err != nil {
			t.Logf("als=%-13s LOAD FAILED: %v", v.name, err)
			continue
		}
		r, err := e.Render(f.Requests["probe"], func(id, payload string) Answer {
			return f.Remotes[id+"/"+payload]
		})
		switch {
		case err != nil:
			t.Logf("als=%-13s RENDER ERROR: %v", v.name, err)
		case !r.Done:
			t.Logf("als=%-13s NEVER SETTLED", v.name)
		case r.Error != "":
			t.Logf("als=%-13s THREW: %s", v.name, firstLine(r.Error))
		case !strings.Contains(r.Body, want):
			t.Logf("als=%-13s WRONG OUTPUT: %s", v.name, r.Body)
		default:
			t.Logf("als=%-13s OK", v.name)
		}

		if leak, lerr := e.LeakedStore(); lerr == nil {
			t.Logf("als=%-13s request store after the render: %s", v.name, leak)
		}

		// The two that must work, work.
		if v.name != "none" {
			if err != nil || !r.Done || r.Error != "" || !strings.Contains(r.Body, want) {
				t.Errorf("als=%s was expected to render the probe page and did not", v.name)
			}
		}
		// ...and the one that must not, does not: if a bare engine with no shim
		// at all started working, the shim is no longer load-bearing and this
		// whole question needs re-asking.
		if v.name == "none" && err == nil && r.Done && r.Error == "" && strings.Contains(r.Body, want) {
			t.Errorf("als=none rendered successfully; svelte no longer requires AsyncLocalStorage " +
				"for async SSR and the shim can be dropped")
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestSequentialRendersOnOneRuntime: the leaked request store must not poison
// the next render on the same Runtime. Each render opens with
// `with_request_store`, which overwrites whatever the last one left behind — so
// reuse should be safe as long as the renders do not overlap. Proven rather
// than assumed, because the hydration payload is the thing that would silently
// go missing.
func TestSequentialRendersOnOneRuntime(t *testing.T) {
	p, f := load(t)
	e, err := New(p.p)
	if err != nil {
		t.Fatal(err)
	}
	answer := func(id, payload string) Answer { return f.Remotes[id+"/"+payload] }

	for i := range 5 {
		r, err := e.Render(f.Requests["probe"], answer)
		if err != nil {
			t.Fatalf("render %d: %v", i, err)
		}
		if !strings.Contains(r.Body, `<p data-testid="site-name">Anvil and Ampersand</p>`) {
			t.Fatalf("render %d lost its content:\n%s", i, r.Body)
		}
		q := r.Data["q"]
		for _, key := range []string{"ezl04l/getSite/", "16h93j5/getTodos/"} {
			if _, ok := q[key]; !ok {
				t.Errorf("render %d: hydration payload lost %q (keys: %v). "+
					"The leaked request store from the previous render is being picked up "+
					"by a query wrapper, so kit thinks the query is nested and omits it.",
					i, key, keys(q))
			}
		}
	}
}

// TestWebcontainerVariantMatches: svelte's own no-AsyncLocalStorage fallback and
// the hand-written shim must produce the same bytes, or one of them is wrong.
func TestWebcontainerVariantMatches(t *testing.T) {
	dir := spikeDir(t)
	_, f := load(t)

	out := map[string]string{}
	for _, file := range []string{"ssr.js", "ssr.webcontainer.js"} {
		prog, err := Program(filepath.Join(dir, "dist", file))
		if err != nil {
			t.Fatal(err)
		}
		e, err := New(prog)
		if err != nil {
			t.Fatal(err)
		}
		var bodies strings.Builder
		for _, name := range []string{"probe", "account", "error", "home", "todos"} {
			r, err := e.Render(f.Requests[name], func(id, payload string) Answer {
				return f.Remotes[id+"/"+payload]
			})
			if err != nil {
				t.Fatalf("%s/%s: %v", file, name, err)
			}
			if !r.Done || r.Error != "" {
				t.Fatalf("%s/%s: done=%v error=%s", file, name, r.Done, r.Error)
			}
			bodies.WriteString(r.Body)
		}
		out[file] = bodies.String()
	}
	if out["ssr.js"] != out["ssr.webcontainer.js"] {
		t.Errorf("the AsyncLocalStorage shim and svelte's webcontainer fallback disagree")
	} else {
		t.Logf("identical output from both, %d bytes over 5 pages", len(out["ssr.js"]))
	}
}
