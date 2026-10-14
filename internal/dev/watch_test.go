package dev

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestFingerprintCountsAuthoredInputsAndNothingTheToolsWrite(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web")
	out := filepath.Join(root, "internal", "skgo")
	put := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	remove := func(rel string) {
		t.Helper()
		if err := os.RemoveAll(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	put("go.mod", "module x\n")
	put("web/src/routes/+page.svelte", "<h1>one</h1>")
	put("web/src/routes/page.server.go", "package routes\n")
	in := inputs{root: root, web: web, out: out}
	print := func() string {
		t.Helper()
		got, err := in.fingerprint()
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	base := print()

	// Output of the generator and the compiler, and files that are not inputs.
	put("web/src/routes/skgo_gen.go", "package routes\n")
	put("web/src/routes/+page.server.ts", "throw new Error('skgo')")
	put("web/src/routes/go.mod", "module routes\n")
	put("web/src/routes/todos.remote.ts", "export {}")
	put("internal/skgo/links/abc/page.server.go", "package x\n")
	put("internal/skgo/skgo_gen.go", "package skgo\n")
	put("web/node_modules/pkg/index.go", "package pkg\n")
	put("web/build/app.go", "package build\n")
	put("web/.svelte-kit/generated.go", "package k\n")
	put("answer_test.go", "package x\n")
	put("web/src/routes/+page.svelte", "<h1>two, hot updated</h1>")
	if got := print(); got != base {
		t.Fatal("something the developer did not author changed the fingerprint")
	}

	steps := []struct {
		name string
		edit func()
	}{
		{"Go body", func() { put("web/src/routes/page.server.go", "package routes\n\nvar x = 1\n") }},
		{"a new Go file", func() { put("web/src/routes/new/page.server.go", "package new\n") }},
		{"a new component", func() { put("web/src/routes/new/+page.svelte", "") }},
		{"a universal module", func() { put("web/src/routes/+page.ts", "export const prerender = true") }},
		{"go.mod", func() { put("go.mod", "module y\n") }},
		{"removing the route", func() { remove("web/src/routes/new") }},
	}
	seen := map[string]string{base: "start"}
	for _, step := range steps {
		step.edit()
		got := print()
		if previous, dup := seen[got]; dup {
			t.Fatalf("%s left the fingerprint as it was after %s", step.name, previous)
		}
		seen[got] = step.name
	}
}

func TestAuthoredPathsPointAtTheFileTheDeveloperWrote(t *testing.T) {
	root := filepath.FromSlash("/work/app")
	web := filepath.Join(root, "web")
	// "src/routes/stream" and "src/routes/(marketing)/pricing" as the generator names them.
	report := strings.Join([]string{
		"# fixture/internal/skgo/links/onzggl3sn52xizltf5zxi4tfmfwq",
		"internal/skgo/links/onzggl3sn52xizltf5zxi4tfmfwq/page.server.go:79:14: syntax error: unexpected {",
		"/work/app/internal/skgo/links/onzggl3sn52xizltf4ug2ylsnnsxi2lom4us64dsnfrws3th/pricing.remote.go:3:1: undefined: x",
		"businesslogic/session.go:12:2: untouched",
		"docs/links/overview/readme.go:1:1: also untouched",
	}, "\n")
	got := authoredPaths(report, root, web)
	for _, want := range []string{
		"# web/src/routes/stream\n",
		"web/src/routes/stream/page.server.go:79:14: syntax error",
		"web/src/routes/(marketing)/pricing/pricing.remote.go:3:1: undefined: x",
		"businesslogic/session.go:12:2: untouched",
		"docs/links/overview/readme.go:1:1: also untouched",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "onzggl3sn52") {
		t.Errorf("a link-tree name survived:\n%s", got)
	}
}

func TestFingerprintReusesBytesAndDetectsPreservedMtimeEdits(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "authored.go")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("package fixture\nconst X=1\n")
	original, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	cache := &fileDigests{}
	in := inputs{root: root, web: filepath.Join(root, "web"), digests: cache}
	fingerprint := func() string {
		t.Helper()
		sum, err := in.fingerprint()
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	first := fingerprint()
	for i := 0; i < 4; i++ {
		if got := fingerprint(); got != first {
			t.Fatal("unchanged source changed fingerprint")
		}
	}
	if !changeTime(original).IsZero() && cache.reads != 1 {
		t.Fatalf("unchanged source reread %d times, want one initial read", cache.reads)
	}
	write("package fixture\nconst X=2\n")
	if err := os.Chtimes(file, original.ModTime(), original.ModTime()); err != nil {
		t.Fatal(err)
	}
	second := fingerprint()
	if second == first {
		t.Fatal("equal-size preserved-mtime write was hidden by cached digest")
	}
	replacement := filepath.Join(root, "replacement.tmp")
	if err := os.WriteFile(replacement, []byte("package fixture\nconst X=3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, original.ModTime(), original.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, file); err != nil {
		t.Fatal(err)
	}
	third := fingerprint()
	if third == second || third == first {
		t.Fatal("atomic same-size preserved-mtime replacement ignored")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if fingerprint() == third {
		t.Fatal("deleted source ignored")
	}
	if len(cache.entries) != 0 {
		t.Fatal("removed source remains in retained digest cache")
	}
	write("package fixture\nconst X=4\n")
	if fingerprint() == third {
		t.Fatal("recreated source ignored")
	}
}
