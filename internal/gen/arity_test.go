package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A remote function that takes no argument is an ordinary Go function that
// takes no argument. Kit's `query(fn)` accepts `(arg?) => Output` — the
// argument is optional and there is no placeholder type — so the marker takes
// `any` and the arity is read from the declaration itself.
//
// Both halves of the projection have to agree with that reading. The stub is
// kit's no-validator overload, which is not cosmetic: `create_validator` reads
// the factory's arity, and the one-argument form installs the validator that
// answers 400 to any argument but `undefined`. The registration is the
// no-argument constructor, which is where the compiler checks the function
// again.
func TestANoArgumentRemoteFunctionIsAnOrdinaryGoFunction(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context) (string, error) {
	return "", nil
}

func doThing(ctx context.Context) (string, error) {
	return "", nil
}

func watchThing(ctx context.Context, yield func(int) error) error {
	return nil
}

func getNamed(ctx context.Context, name string) (string, error) {
	return "", nil
}

var (
	_ = skgo.Query(getThing)
	_ = skgo.Command(doThing)
	_ = skgo.LiveQuery(watchThing)
	_ = skgo.Query(getNamed)
)
`, nil)

	if err := Run(cfg); err != nil {
		t.Fatalf("generating an app whose remote functions take no argument: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.ts")
	for _, want := range []string{
		"export const getThing = query((): string => unimplemented());",
		"export const doThing = command((): string => unimplemented());",
		"export const watchThing = query.live((): AsyncIterable<number> => unimplemented());",
		// The one that does take an argument still declares it, and still says
		// `'unchecked'`: without a validator kit refuses every argument.
		"export const getNamed = query('unchecked', (_arg: string): string => unimplemented());",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("the stub does not carry %q:\n%s", want, stub)
		}
	}

	bindings := readFixtureFile(t, root, "app/web/src/data/skgo_remotes_gen.go")
	for _, want := range []string{
		`skgo.NewQueryNoArg("src/data/data.remote.ts", "getThing", getThing)`,
		`skgo.NewCommandNoArg("src/data/data.remote.ts", "doThing", doThing)`,
		`skgo.NewLiveQueryNoArg("src/data/data.remote.ts", "watchThing", watchThing)`,
		`skgo.NewQuery("src/data/data.remote.ts", "getNamed", getNamed)`,
	} {
		if !strings.Contains(bindings, want) {
			t.Errorf("the registration file does not carry %q:\n%s", want, bindings)
		}
	}

	// The registration is where the shape is checked now, so it has to compile.
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("`go build ./...` in the generated app: %v\n%s", err, out)
	}
}

// No skgo type reaches the type projector any more. `skgo.None` was the only
// one that ever did — it was a remote function's argument, so it was projected
// like any other — and a placeholder that has to be projected is a placeholder
// that can fail to be.
func TestNoArgumentMeansNothingToProject(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

func getThing(ctx context.Context) (string, error) {
	return "", nil
}

var _ = skgo.Query(getThing)
`, nil)

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.ts")
	if strings.Contains(stub, "tylergannon/skgo") {
		t.Errorf("the stub imports a declaration of an skgo type:\n%s", stub)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "generated", "wiretypes")); !os.IsNotExist(err) {
		t.Errorf("a declaration package was written for a type skgo owns: %v", err)
	}
}

// The marker takes `any`, so it is `skgo generate` that has to report a
// function it cannot publish — at the declaration's own position, saying what
// the shape should have been. A message that only says "cannot read the types"
// leaves the developer to guess.
func TestAFunctionSkgoCannotPublishIsRefusedByName(t *testing.T) {
	cases := []struct {
		name    string
		decl    string
		wantAll []string
	}{
		{
			name: "no context at all",
			decl: `func getThing(id string) (string, error) { return "", nil }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"getThing", "query", "func(context.Context) (Out, error)"},
		},
		{
			name: "no error result",
			decl: `func getThing(ctx context.Context) string { return "" }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"getThing", "query", "(Out, error)"},
		},
		{
			name: "two arguments",
			decl: `func getThing(ctx context.Context, a string, b string) (string, error) { return "", nil }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"getThing", "query"},
		},
		{
			name: "variadic",
			decl: `func getThing(ctx context.Context, ids ...string) (string, error) { return "", nil }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"getThing", "variadic"},
		},
		{
			name: "a live query with no yield",
			decl: `func watchThing(ctx context.Context) error { return nil }

var _ = skgo.LiveQuery(watchThing)`,
			wantAll: []string{"watchThing", "query.live", "func(Out) error"},
		},
		{
			name: "a live query that returns a value",
			decl: `func watchThing(ctx context.Context, yield func(int) error) (int, error) { return 0, nil }

var _ = skgo.LiveQuery(watchThing)`,
			wantAll: []string{"watchThing", "query.live"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cfg := foreignFixture(t, "package data\n\nimport (\n\t\"context\"\n\n\t\"github.com/tylergannon/skgo\"\n)\n\nvar _ = context.Background\n\n"+tc.decl+"\n", nil)

			err := Run(cfg)
			if err == nil {
				t.Fatal("the generator published a function whose signature it cannot call")
			}
			for _, want := range tc.wantAll {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not mention %q:\n%v", want, err)
				}
			}
			t.Logf("refused with:\n%v", err)
		})
	}
}
