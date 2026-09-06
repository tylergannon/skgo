package gen

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The generator projects a named type by writing a polytype registration file
// into the package that declares it, and then having polytype write a
// `jsonschema/` directory beside it. That is only skgo's to do inside the app's
// own module. A consumer gets skgo, and every other dependency, read-only from
// the module cache; in a workspace where a dependency happens to be writable,
// generating would quietly edit source the app does not own.
//
// These two tests hold the line at the filesystem: whatever the generator
// decides, nothing may appear outside the app's module.

// foreignFixture lays out three modules in a temp dir: the app, a library it
// depends on, and a stand-in for skgo at skgo's own import path. Markers are
// recognised by import path, so the stand-in is what the scanner sees in a
// downstream project, without dragging the real module (or its dependencies,
// or a network) into the test.
//
// remote is the body of `<app>/web/src/data/data.remote.go`.
func foreignFixture(t *testing.T, remote string) (root string, cfg Config) {
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

import "context"

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
`)

	write("wire/go.mod", "module example.com/wire\n\ngo 1.27.1\n")
	write("wire/wire.go", "package wire\n\ntype Thing struct {\n\tName string `json:\"name\"`\n}\n")

	write("app/go.mod", `module example.com/app

go 1.27.1

require (
	example.com/wire v0.0.0
	github.com/tylergannon/skgo v0.0.0
)

replace example.com/wire => ../wire

replace github.com/tylergannon/skgo => ../skgo
`)
	write("app/web/src/data/data.remote.go", remote)
	if err := os.MkdirAll(filepath.Join(root, "app", "generated"), 0o755); err != nil {
		t.Fatal(err)
	}

	return root, Config{
		Web:  filepath.Join(root, "app", "web"),
		Out:  filepath.Join(root, "app", "generated"),
		Logf: func(string, ...any) {},
	}
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

// TestATypeFromAnotherModuleIsRefused: a remote function whose result is a
// named type declared by a dependency cannot be projected, because polytype's
// registration has to be a method on the type, in the type's own package —
// source the app has no right to write. skgo has to say so instead of writing
// it anyway.
func TestATypeFromAnotherModuleIsRefused(t *testing.T) {
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
`)
	before := filesOutsideTheApp(t, root)

	err := Run(cfg)

	requireNothingWrittenOutsideTheApp(t, root, before)

	if err == nil {
		t.Fatal("the generator accepted a type it cannot declare without editing another module")
	}
	for _, want := range []string{"wire.Thing", "example.com/wire", "example.com/app"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not mention %q:\n%v", want, err)
		}
	}
	t.Logf("refused with:\n%v", err)
}

// TestASkgoNoneResultIsNotAWireType: skgo.None is skgo's own marker for
// "nothing here". As an argument it already means kit's no-argument overload;
// as a result it means a remote function that returns nothing, which is
// `void` on the client. It is never a type to declare, and it lives in skgo's
// module, so declaring it would write into skgo itself.
func TestASkgoNoneResultIsNotAWireType(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

func ping(ctx context.Context, _ skgo.None) (skgo.None, error) {
	return skgo.None{}, nil
}

var _ = skgo.Command(ping)
`)
	before := filesOutsideTheApp(t, root)

	err := Run(cfg)

	requireNothingWrittenOutsideTheApp(t, root, before)

	if err != nil {
		t.Fatalf("generating a command that returns nothing: %v", err)
	}
	stub, err := os.ReadFile(filepath.Join(root, "app", "web", "src", "data", "data.remote.ts"))
	if err != nil {
		t.Fatalf("reading the generated stub: %v", err)
	}
	if !strings.Contains(string(stub), "command((): void => unimplemented())") {
		t.Fatalf("a command returning skgo.None is not typed as returning nothing:\n%s", stub)
	}
}
