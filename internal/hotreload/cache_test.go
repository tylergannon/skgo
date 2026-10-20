//go:build skgo_qualification

package hotreload

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/gen"
	"golang.org/x/mod/modfile"
)

func TestHostCacheResolvesUnchangedModulesAtTheNextRevisionsPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", "off")
	app := filepath.Join(work, "app")
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mod, err := os.ReadFile(filepath.Join(root, "example/go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", strings.ReplaceAll(strings.ReplaceAll(string(mod), "module github.com/tylergannon/skgo/example", "module example.com/cached"), "=> ../", "=> "+filepath.ToSlash(root)))
	sums, err := os.ReadFile(filepath.Join(root, "example/go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(sums))
	write("web/package.json", `{"name":"host-cache","type":"module"}`)
	write("web/src/app.html", "<!doctype html>")
	write("locals/locals.go", "package locals\ntype Locals struct{}\n")
	write("server.go", "package cached\nimport \"github.com/tylergannon/skgo\"\nvar Receipt = skgo.FetchConfig{Origin: \"one\"}\n")

	project, err := gen.OpenProject(gen.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/cached/locals", LocalsType: "Locals", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer project.Close()
	prepare := func() *gen.ProjectBuild {
		t.Helper()
		b, e := project.Prepare(ctx)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { b.Cancel(); b.Inputs.Close() })
		return b
	}
	cache := &HostCache{Dir: filepath.Join(work, "hosts")}
	// Without a go command, only a cached resolution can succeed.
	resolve := func(b *gen.ProjectBuild, view string, withGo bool, packages ...string) (Toolchain, error) {
		t.Helper()
		tc, e := prepareToolchain(ctx, "go", filepath.Join(cache.Dir, "toolchain"), b.Inputs.Env, cache)
		if e != nil {
			t.Fatal(e)
		}
		if tc, e = tc.hostOverlay(b.Inputs.Dir, filepath.Join(work, view), map[string][]byte{}); e != nil {
			t.Fatal(e)
		}
		if !withGo {
			tc.Go = filepath.Join(work, "missing-go")
		}
		return cache.resolveModules(ctx, tc, b.Inputs, filepath.Join(work, view), packages)
	}

	first := prepare()
	if _, err := resolve(first, "first", true, "example.com/cached"); err != nil {
		t.Fatal(err)
	}
	write("server.go", "package cached\nimport \"github.com/tylergannon/skgo\"\nvar Receipt = skgo.FetchConfig{Origin: \"two\"}\n")
	if _, _, err := project.Observe(ctx, filepath.Join(app, "server.go")); err != nil {
		t.Fatal(err)
	}
	second := prepare()
	want := second.Inputs.PrivatePath(root)
	if want == first.Inputs.PrivatePath(root) || want == root {
		t.Fatalf("revisions do not materialize skgo at distinct private paths: %s", want)
	}
	tc, err := resolve(second, "second", false, "example.com/cached")
	if err != nil {
		t.Fatalf("unchanged modules were resolved again: %v", err)
	}
	data, err := os.ReadFile(tc.ModFile)
	if err != nil {
		t.Fatal(err)
	}
	f, err := modfile.Parse(tc.ModFile, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, r := range f.Replace {
		if r.Old.Path == "github.com/tylergannon/skgo" {
			got = r.New.Path
		}
	}
	if got != want {
		t.Fatalf("host module replaces skgo with %q, want the second revision's copy %q", got, want)
	}
	if _, err := resolve(second, "third", false, "example.com/cached", "example.com/cached/other"); err == nil {
		t.Fatal("a different package set reused the previous resolution")
	}
}
