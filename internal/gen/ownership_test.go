package gen

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// Projecting a named type means writing a polytype registration file into a Go
// package, and then having polytype write `jsonschema_gen.go` and a
// `jsonschema/` directory beside it. For a type the app declares, that package
// is the app's own. For a type from a dependency it is the module cache: source
// the app does not own and, for any real consumer, cannot write.
//
// These tests hold the line at the filesystem. A named type from another module
// has to reach the browser, and nothing may appear outside the app's module
// while it does.

// foreignFixture lays out three modules in a temp dir: the app, a library it
// depends on, and a stand-in for skgo at skgo's own import path. Markers are
// recognised by import path, so the stand-in is what the scanner sees in a
// downstream project, without dragging the real module (or its dependencies,
// or a network) into the test.
//
// remote is the body of `<app>/web/src/data/data.remote.go`; extra adds any
// further files, keyed by their slash-separated path under the fixture root.
func foreignFixture(t *testing.T, remote string, extra map[string]string) (root string, cfg Config) {
	t.Helper()
	// The fixture is not a member of this repository's workspace, so the go
	// commands the generator runs have to see the fixture's own go.mod.
	t.Setenv("GOWORK", "off")

	root = t.TempDir()
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("skgo/go.mod", "module github.com/tylergannon/skgo\n\ngo 1.27.1\n")
	write("skgo/skgo.go", `package skgo

import (
	"context"
	"net/http"
)

type None struct{}

type Marker struct{}

type Remote struct{}

func Query[In, Out any](fn func(context.Context, In) (Out, error)) Marker { _ = fn; return Marker{} }

func Command[In, Out any](fn func(context.Context, In) (Out, error)) Marker { _ = fn; return Marker{} }

func LiveQuery[In, Out any](fn func(context.Context, In, func(Out) error) error) Marker {
	_ = fn
	return Marker{}
}

func NewQuery[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	_, _, _ = module, name, fn
	return &Remote{}
}

func NewCommand[In, Out any](module, name string, fn func(context.Context, In) (Out, error)) *Remote {
	_, _, _ = module, name, fn
	return &Remote{}
}

func NewLiveQuery[In, Out any](module, name string, fn func(context.Context, In, func(Out) error) error) *Remote {
	_, _, _ = module, name, fn
	return &Remote{}
}

type ServerLoad struct{}

func NewLoad[Out any](module string, fn func(context.Context) (Out, error)) *ServerLoad {
	_, _ = module, fn
	return &ServerLoad{}
}

type Endpoint struct{}

func GET(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func POST(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func NewEndpoint(routeID, method string, fn http.HandlerFunc) *Endpoint {
	_, _, _ = routeID, method, fn
	return &Endpoint{}
}
`)

	// `wire` is the dependency. Its types are the ones that have to travel: a
	// struct, and an enum reached only through that struct's field.
	write("wire/go.mod", "module example.com/wire\n\ngo 1.27.1\n")
	write("wire/wire.go", `package wire

type Thing struct {
	Name   string `+"`json:\"name\"`"+`
	Status Status `+"`json:\"status\"`"+`
}

type Status string

func (Status) enum() {}

const (
	StatusOn  Status = "on"
	StatusOff Status = "off"
)
`)

	// A second dependency package, also called `wire` and also declaring a
	// `Thing`. Two libraries picking the same short name is an ordinary thing
	// to have, and neither name is the app's to change.
	write("wire/other/wire/wire.go", "package wire\n\ntype Thing struct {\n\tLabel string `json:\"label\"`\n}\n")

	goMod, sum := fixtureModule(t)
	write("app/go.mod", goMod)
	write("app/go.sum", sum)
	if err := os.MkdirAll(filepath.Join(root, "app", "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range extra {
		write(path, content)
	}
	if remote != "" {
		write("app/web/src/data/data.remote.go", remote)
	}

	return root, Config{
		Web:  filepath.Join(root, "app", "web"),
		Out:  filepath.Join(root, "app", "generated"),
		Logf: func(string, ...any) {},
	}
}

// fixtureModule builds the fixture app's go.mod out of the example app's, so
// the fixture runs the polytype this repository actually pins rather than a
// second copy of the version number. The generator shells out to `go tool
// polytype`, so the fixture has to be a module that can run it.
func fixtureModule(t *testing.T) (goMod, goSum string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "example", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := modfile.Parse("go.mod", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("module example.com/app\n\n")
	fmt.Fprintf(&b, "go %s\n\nrequire (\n", parsed.Go.Version)
	b.WriteString("\texample.com/wire v0.0.0\n")
	for _, req := range parsed.Require {
		if req.Mod.Path == skgoPkg {
			continue
		}
		fmt.Fprintf(&b, "\t%s %s\n", req.Mod.Path, req.Mod.Version)
	}
	fmt.Fprintf(&b, "\t%s v0.0.0\n)\n\n", skgoPkg)
	b.WriteString("tool github.com/tylergannon/polytype/polytype\n\n")
	b.WriteString("replace example.com/wire => ../wire\n\n")
	fmt.Fprintf(&b, "replace %s => ../skgo\n", skgoPkg)

	sum, err := os.ReadFile(filepath.Join("..", "..", "example", "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	return b.String(), string(sum)
}

// filesOutsideTheApp lists every file in the fixture that does not belong to
// the app's own module.
func filesOutsideTheApp(t *testing.T, root string) []string {
	t.Helper()
	app := filepath.Join(root, "app") + string(filepath.Separator)
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(path, app) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func requireNothingWrittenOutsideTheApp(t *testing.T, root string, before []string) {
	t.Helper()
	was := map[string]bool{}
	for _, f := range before {
		was[f] = true
	}
	var added []string
	for _, f := range filesOutsideTheApp(t, root) {
		if !was[f] {
			added = append(added, f)
		}
	}
	if len(added) > 0 {
		t.Fatalf("the generator wrote into modules the app does not own:\n\t%s", strings.Join(added, "\n\t"))
	}
}

// TestATypeFromAnotherModuleCrossesTheWire is the whole point of the exercise.
// A remote function whose result is a named type declared by a dependency is an
// ordinary thing to write — a domain type from a shared module, a uuid.UUID —
// and it has to reach the browser with its real name on it, without the
// generator touching the dependency's own source.
func TestATypeFromAnotherModuleCrossesTheWire(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"example.com/wire"
	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context, _ skgo.None) (wire.Thing, error) {
	return wire.Thing{}, nil
}

var _ = skgo.Query(getThing)

func getStatus(ctx context.Context, name string) (wire.Status, error) {
	return wire.StatusOn, nil
}

var _ = skgo.Query(getStatus)
`, nil)
	before := filesOutsideTheApp(t, root)

	if err := Run(cfg); err != nil {
		t.Fatalf("generating an app whose wire types come from a dependency: %v", err)
	}

	requireNothingWrittenOutsideTheApp(t, root, before)

	// The declaration a caller needs: the dependency's types, under their own
	// names, including the enum reached only through a field.
	ts := readFixtureFile(t, root, "app/web/src/lib/skgo/example.com/wire/types.ts")
	for _, want := range []string{"export type Thing = {", `"name": string;`, `"status": Status;`, `export type Status = "on" | "off";`} {
		if !strings.Contains(ts, want) {
			t.Fatalf("the generated TypeScript does not declare the dependency's types (%q missing):\n%s", want, ts)
		}
	}

	// And the stub a caller imports, typed with those same names.
	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.ts")
	for _, want := range []string{
		"import type { Status, Thing } from '../lib/skgo/example.com/wire/types';",
		"export const getThing = query((): Thing => unimplemented());",
		"export const getStatus = query('unchecked', (_arg: string): Status => unimplemented());",
	} {
		if !strings.Contains(stub, want) {
			t.Fatalf("the stub does not use the dependency's types (%q missing):\n%s", want, stub)
		}
	}

	// The generated app has to be a Go program, in both of the shapes it is
	// ever built in: polytype's own output carries `//go:build !jsonschema`, so
	// a declaration that exists only under the tag leaves an ordinary build
	// broken.
	app := filepath.Join(root, "app")
	for _, args := range [][]string{{"build", "./..."}, {"build", "-tags", "jsonschema", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = app
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("`go %s` in the generated app: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// TestTwoDependenciesCalledTheSameThingAreKeptApart. Package names are short
// and not unique, and a dependency's is not the app's to change. Import paths
// are the only names guaranteed to differ, so they are what addresses both
// halves of the projection: the declaration package in the app's module, and
// the types.ts the stubs import.
func TestTwoDependenciesCalledTheSameThingAreKeptApart(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"example.com/wire"
	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context, _ skgo.None) (wire.Thing, error) {
	return wire.Thing{}, nil
}

var _ = skgo.Query(getThing)
`, map[string]string{
		"app/web/src/other/other.remote.go": `package other

import (
	"context"

	wire "example.com/wire/other/wire"
	"github.com/tylergannon/skgo"
)

func getOtherThing(ctx context.Context, _ skgo.None) (wire.Thing, error) {
	return wire.Thing{}, nil
}

var _ = skgo.Query(getOtherThing)
`,
	})
	before := filesOutsideTheApp(t, root)

	if err := Run(cfg); err != nil {
		t.Fatalf("generating an app that depends on two packages called wire: %v", err)
	}
	requireNothingWrittenOutsideTheApp(t, root, before)

	// Each package's own Thing, at its own address, with its own fields.
	first := readFixtureFile(t, root, "app/web/src/lib/skgo/example.com/wire/types.ts")
	if !strings.Contains(first, `"name": string;`) {
		t.Fatalf("example.com/wire projected the wrong Thing:\n%s", first)
	}
	second := readFixtureFile(t, root, "app/web/src/lib/skgo/example.com/wire/other/wire/types.ts")
	if !strings.Contains(second, `"label": string;`) {
		t.Fatalf("example.com/wire/other/wire projected the wrong Thing:\n%s", second)
	}

	// And two declaration packages in the app, likewise addressed by import
	// path rather than by the name both packages share.
	for _, rel := range []string{
		"app/generated/wiretypes/example.com/wire/skgo_wiretypes_gen.go",
		"app/generated/wiretypes/example.com/wire/other/wire/skgo_wiretypes_gen.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("no declaration package at %s: %v", rel, err)
		}
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("`go build ./...` in the generated app: %v\n%s", err, out)
	}
}

// TestOneStubCannotImportTwoTypesOfTheSameName. Separate directories stop
// being enough once both types reach the same `.remote.ts`: TypeScript has one
// namespace per module, and a stub refers to every type by the name its own
// package gave it. Say so rather than emitting a module that declares `Thing`
// twice.
func TestOneStubCannotImportTwoTypesOfTheSameName(t *testing.T) {
	_, cfg := foreignFixture(t, `package data

import (
	"context"

	"example.com/wire"
	otherwire "example.com/wire/other/wire"
	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context, _ skgo.None) (wire.Thing, error) {
	return wire.Thing{}, nil
}

var _ = skgo.Query(getThing)

func getOtherThing(ctx context.Context, _ skgo.None) (otherwire.Thing, error) {
	return otherwire.Thing{}, nil
}

var _ = skgo.Query(getOtherThing)
`, nil)

	err := Run(cfg)
	if err == nil {
		t.Fatal("the generator emitted a stub that imports two different types called Thing")
	}
	for _, want := range []string{"Thing", "example.com/wire", "example.com/wire/other/wire"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not mention %q:\n%v", want, err)
		}
	}
	t.Logf("refused with:\n%v", err)
}

// TestADeclarationPackageGoesWithTheDependencyThatNeededIt. A declaration
// package is ordinary Go source in the app's module, so one left behind for a
// dependency that is no longer on the wire is a build failure rather than
// clutter — it still imports a module the app may since have dropped.
func TestADeclarationPackageGoesWithTheDependencyThatNeededIt(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"example.com/wire"
	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context, _ skgo.None) (wire.Thing, error) {
	return wire.Thing{}, nil
}

var _ = skgo.Query(getThing)
`, nil)

	if err := Run(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	declared := filepath.Join(root, "app", "generated", "wiretypes")
	if _, err := os.Stat(declared); err != nil {
		t.Fatalf("nothing was declared for the dependency: %v", err)
	}

	// The same app, with the dependency taken back off the wire.
	if err := os.WriteFile(filepath.Join(root, "app", "web", "src", "data", "data.remote.go"), []byte(`package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context, _ skgo.None) (string, error) {
	return "", nil
}

var _ = skgo.Query(getThing)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, err := os.Stat(declared); !os.IsNotExist(err) {
		t.Fatalf("the declaration package outlived the dependency that needed it: %v", err)
	}
}

func readFixtureFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(raw)
}
