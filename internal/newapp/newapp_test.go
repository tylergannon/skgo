package newapp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/newapp"
)

// scaffold writes a project with everything pinned, so a test can look at the
// files without a toolchain, a network or a build.
func scaffold(t *testing.T, o newapp.Options) string {
	t.Helper()
	if o.Dir == "" {
		o.Dir = filepath.Join(t.TempDir(), "myapp")
	}
	if o.SkgoVersion == "" {
		o.SkgoVersion = scaffoldVersion
	}
	if err := newapp.Create(o); err != nil {
		t.Fatalf("scaffolding: %v", err)
	}
	return o.Dir
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestTheOriginIsWrittenDownOnce is the trap this template exists to close.
//
// The origin is fixed when the frontend is built and checked again on every
// non-GET remote call, so a project whose two halves were given different
// origins answers every command with 403 and explains nothing. The template
// takes the origin once and puts the same string everywhere it is needed; if
// one of those places is ever filled in by hand, this test is what notices.
func TestTheOriginIsWrittenDownOnce(t *testing.T) {
	const origin = "https://app.example.com"
	dir := scaffold(t, newapp.Options{Origin: origin})

	// The frontend is built with it, the binary is linked with it, and the
	// developer edits it in exactly one file.
	for _, file := range []string{"mise.toml", "web/vite.config.ts", "cmd/main.go", "README.md"} {
		if !strings.Contains(read(t, dir, file), origin) {
			t.Errorf("%s does not carry the app's origin", file)
		}
	}
	if def := read(t, dir, "cmd/main.go"); !strings.Contains(def, `"app.example.com"`) {
		t.Error("cmd/main.go does not default its listen address to the origin's host and port")
	}
	// The default has to be gone, not merely outvoted: a leftover
	// 127.0.0.1:8080 in one of the two halves is the failure itself.
	for _, file := range []string{"mise.toml", "web/vite.config.ts", "cmd/main.go"} {
		if strings.Contains(read(t, dir, file), "127.0.0.1:8080") {
			t.Errorf("%s still carries the default origin as well as %s", file, origin)
		}
	}
}

func TestTheProjectIsNamedThroughout(t *testing.T) {
	dir := scaffold(t, newapp.Options{
		Dir:    filepath.Join(t.TempDir(), "guestbook"),
		Module: "github.com/example/guestbook",
	})

	if mod := read(t, dir, "go.mod"); !strings.Contains(mod, "module github.com/example/guestbook\n") {
		t.Errorf("go.mod does not declare the module path:\n%s", mod)
	}
	if main := read(t, dir, "cmd/main.go"); !strings.Contains(main, `"github.com/example/guestbook/web"`) {
		t.Errorf("cmd/main.go does not import the app's own packages:\n%s", main)
	}
	if remote := read(t, dir, "web/src/routes/hello.remote.go"); !strings.Contains(remote, `Name:         "guestbook"`) {
		t.Error("the demo remote function does not answer with the project's name")
	}
	if mise := read(t, dir, "mise.toml"); !strings.Contains(mise, "-o bin/guestbook ./cmd") {
		t.Error("the build task does not name the binary after the project")
	}
}

// TestTheProjectRequiresSkgoAsADependency guards the one property that makes
// this a scaffold rather than a copy of the example: the new project stands on
// its own.
func TestTheProjectRequiresSkgoAsADependency(t *testing.T) {
	dir := scaffold(t, newapp.Options{})
	mod := read(t, dir, "go.mod")

	for _, want := range []string{
		"github.com/tylergannon/skgo " + scaffoldVersion,
		"github.com/tylergannon/polytype ",
		"github.com/tylergannon/skgo/cmd/skgo",
	} {
		if !strings.Contains(mod, want) {
			t.Errorf("go.mod does not carry %q:\n%s", want, mod)
		}
	}
	if strings.Contains(mod, "replace") {
		t.Errorf("go.mod has a replace directive:\n%s", mod)
	}
	// polytype is driven as a library from inside skgo; nothing in the project
	// runs its CLI, and a tool directive would pull its dependencies into the
	// project's go.sum for nothing.
	if strings.Contains(mod, "github.com/tylergannon/polytype/polytype") {
		t.Errorf("go.mod runs polytype as a tool:\n%s", mod)
	}
	// `go tool skgo` builds the generator from the module cache, where nothing
	// is writable. A directive that assumed a checkout would work here and
	// only here.
	if gen := read(t, dir, "generated/config.go"); !strings.Contains(gen, "go:generate go tool skgo generate") {
		t.Errorf("generated/config.go does not invoke skgo through `go tool`:\n%s", gen)
	}
}

func TestItRefusesToWriteOverAnExistingProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	scaffold(t, newapp.Options{Dir: dir})

	err := newapp.Create(newapp.Options{Dir: dir, SkgoVersion: scaffoldVersion})
	if err == nil {
		t.Fatal("scaffolding over an existing project succeeded")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
}

func TestItRefusesNamesTheProjectCannotCarry(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    newapp.Options
	}{
		{"an app name that is not a file name", newapp.Options{App: "my app"}},
		{"a module path Go cannot resolve", newapp.Options{Module: "not a module"}},
		{"an origin with a path", newapp.Options{Origin: "http://127.0.0.1:8080/app"}},
		{"an origin with no scheme", newapp.Options{Origin: "127.0.0.1:8080"}},
		{"a version go.mod cannot require", newapp.Options{SkgoVersion: "main"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.o
			o.Dir = filepath.Join(t.TempDir(), "myapp")
			if o.SkgoVersion == "" {
				o.SkgoVersion = scaffoldVersion
			}
			if err := newapp.Create(o); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

// TestTheScaffoldVendorsNoAdapter states the rule the adapter has always
// obeyed: a project never carries a copy of it.
//
// A vendored adapter is a second version of skgo living in the app, and it
// drifts silently — the copy that reached the field was months behind the Go
// beside it and failed the build with an unresolved esbuild import. The adapter
// is now an ordinary npm package, so the scaffold ships what a developer would
// have typed: one devDependency line and the package import.
func TestTheScaffoldVendorsNoAdapter(t *testing.T) {
	dir := scaffold(t, newapp.Options{AdapterSpec: "^1.2.3"})
	if _, err := os.Stat(filepath.Join(dir, "web", "skgo-adapter.js")); err == nil {
		t.Fatal("`skgo new` wrote web/skgo-adapter.js; the adapter belongs to the skgo module, " +
			"and a copy in the project is the drift this was meant to end")
	}
	if config := read(t, dir, "web/vite.config.ts"); !strings.Contains(config, `from '@skgo/sveltekit-adapter'`) {
		t.Fatalf("the scaffolded vite config does not import the adapter package:\n%s", config)
	}
	if got := pin(t, read(t, dir, "web/package.json"), "@skgo/sveltekit-adapter"); got != "^1.2.3" {
		t.Errorf("the scaffold asks npm for @skgo/sveltekit-adapter %q; want the spec it was given, ^1.2.3", got)
	}
}

// TestTheScaffoldAsksForTheAdapterThatMatchesItsSkgo is the pairing rule: the
// npm package and the Go module are one release, and a project that asked for
// any other one could not serve what it built.
func TestTheScaffoldAsksForTheAdapterThatMatchesItsSkgo(t *testing.T) {
	dir := scaffold(t, newapp.Options{SkgoVersion: "v9.4.2"})
	const want = "9.4.2"
	if got := pin(t, read(t, dir, "web/package.json"), "@skgo/sveltekit-adapter"); got != want {
		t.Errorf("a project requiring skgo v9.4.2 asks pnpm for @skgo/sveltekit-adapter %q; want %q", got, want)
	}
}

// TestTheTemplateCarriesTheToolchainTheExampleIsBuiltWith checks the versions a
// new project is pinned to against the ones this repository actually builds
// with. Kit 3 is a moving target; a scaffold pinned to something nobody here
// has ever run is a scaffold nobody has tested.
func TestTheTemplateCarriesTheToolchainTheExampleIsBuiltWith(t *testing.T) {
	root := checkoutRoot(t)
	dir := scaffold(t, newapp.Options{})

	example, err := os.ReadFile(filepath.Join(root, "example", "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	scaffolded := read(t, dir, "web/package.json")

	for _, dep := range []string{"@sveltejs/kit", "svelte", "vite-plus", "typescript", "@sveltejs/vite-plugin-svelte"} {
		want := pin(t, string(example), dep)
		if got := pin(t, scaffolded, dep); got != want {
			t.Errorf("the template pins %s at %s; the example builds with %s", dep, got, want)
		}
	}
}

// pin reads the version a package.json pins a dependency at, without decoding
// the whole file: the point is the literal line a developer reads.
func pin(t *testing.T, packageJSON, dep string) string {
	t.Helper()
	for line := range strings.Lines(packageJSON) {
		key := `"` + dep + `":`
		if !strings.Contains(line, key) {
			continue
		}
		_, rest, _ := strings.Cut(line, key)
		return strings.Trim(strings.TrimSpace(rest), `",`)
	}
	t.Fatalf("no %s in package.json", dep)
	return ""
}
