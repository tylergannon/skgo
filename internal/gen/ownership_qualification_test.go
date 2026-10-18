//go:build skgo_qualification

package gen

import (
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

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
	t.Parallel()
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"example.com/wire"
	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context) (wire.Thing, error) {
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

	// The route directory holds the developer's Go, the stub, the registration
	// that names the handler — and nothing of polytype's. No marker file, no
	// schema, no declaration package for the dependency anywhere in the app.
	if got, want := filesUnder(t, filepath.Join(root, "app", "web", "src", "data")), []string{"data.remote.go", "data.remote.ts", "skgo_gen.go"}; !slices.Equal(got, want) {
		t.Fatalf("the route directory holds %v, want %v", got, want)
	}
	for _, f := range filesUnder(t, filepath.Join(root, "app")) {
		base := filepath.Base(f)
		if strings.Contains(f, "jsonschema") || strings.Contains(f, "wiretypes") || base == "skgo_polytype_gen.go" {
			t.Fatalf("polytype's CLI path left %s in the app", f)
		}
	}

	// The generated app has to be a Go program.
}
