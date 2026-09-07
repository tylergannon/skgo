package ssrspike

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMeasurements prints what an embedded engine would cost per request. Run
// with `-v`; it asserts nothing, because a number is not a verdict.
func TestMeasurements(t *testing.T) {
	dir := spikeDir(t)
	bundle := filepath.Join(dir, "dist", "ssr.js")
	info, err := os.Stat(bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("bundle: %s, %.1f KiB", bundle, float64(info.Size())/1024)

	p, f := load(t)

	// One compile, reused by every Runtime.
	start := time.Now()
	prog, err := Program(bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("goja compile (once, shareable): %v", time.Since(start).Round(time.Microsecond))

	// A fresh Runtime plus a full evaluation of the bundle: the cost of adding a
	// Runtime to a pool, and the cost per request if there is no pool.
	const runs = 20
	start = time.Now()
	for range runs {
		if _, err := New(prog); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("goja fresh Runtime + evaluate bundle: %v each", (time.Since(start) / runs).Round(time.Microsecond))

	for _, name := range []string{"probe", "account", "error", "home"} {
		e, err := New(prog)
		if err != nil {
			t.Fatal(err)
		}
		answer := func(id, payload string) Answer { return f.Remotes[id+"/"+payload] }

		// warm up: first render pays for lazily-created shapes
		for range 5 {
			if _, err := e.Render(f.Requests[name], answer); err != nil {
				t.Fatal(err)
			}
		}
		const n = 50
		start = time.Now()
		for range n {
			if _, err := e.Render(f.Requests[name], answer); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("goja warm render %-8s %v", name, (time.Since(start) / n).Round(time.Microsecond))
	}
	_ = p
}

// TestWriteDocument assembles the document the way Go would — kit's own
// `app.html` with `%sveltekit.body%` replaced by what the engine rendered — and
// writes it out so a human can open it and see the page. It is not a hydrating
// document: the `__sveltekit_*` boot script, the devalue'd data, the CSP and
// the modulepreloads are the rest of Go's job and are not part of this spike.
func TestWriteDocument(t *testing.T) {
	dir := spikeDir(t)
	template, err := os.ReadFile(filepath.Join(dir, "..", "..", "example", "web", "src", "app.html"))
	if err != nil {
		t.Fatal(err)
	}

	p, f := load(t)
	for _, name := range []string{"probe", "account", "error"} {
		e, err := New(p.p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := e.Render(f.Requests[name], func(id, payload string) Answer {
			return f.Remotes[id+"/"+payload]
		})
		if err != nil || !r.Done || r.Error != "" {
			t.Fatalf("%s: err=%v done=%v error=%s", name, err, r.Done, r.Error)
		}

		doc := strings.ReplaceAll(string(template), "%sveltekit.head%", r.Head)
		doc = strings.ReplaceAll(doc, "%sveltekit.body%", r.Body)
		if strings.Contains(doc, "%sveltekit.") {
			t.Fatalf("%s: app.html has a placeholder this spike does not fill: %s", name, doc)
		}
		out := filepath.Join(dir, "dist", name+".html")
		if err := os.WriteFile(out, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d bytes)", out, len(doc))
	}
}
