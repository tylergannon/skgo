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

	// The registration names the kind and the module; the handler beside it is
	// where the arity shows, because that is where the argument is refused or
	// required. Kit draws the same line: `create_validator` reads the arity and
	// installs the refusal.
	bindings := readFixtureFile(t, root, "app/generated/skgo_bindings_gen.go")
	for _, want := range []string{
		`skgo.KindQuery,`,
		`skgo.KindCommand,`,
		`skgo.KindLive,`,
		`"getThing",`,
		`"doThing",`,
		`"watchThing",`,
		`"getNamed",`,
		// No argument: any argument at all is refused.
		"func remote_getThing(ctx context.Context, call skgo.Call) (any, error) {\n\tif err := skgo.RefuseArgument(call); err != nil {",
		"func remote_doThing(ctx context.Context, call skgo.Call) (any, error) {\n\tif err := skgo.RefuseArgument(call); err != nil {",
		// An argument: it is required, and decoded by the codec generated for
		// its own type.
		"func remote_getNamed(ctx context.Context, call skgo.Call) (any, error) {\n\tif err := skgo.RequireArgument(call); err != nil {",
	} {
		if !strings.Contains(bindings, want) {
			t.Errorf("the bindings file does not carry %q:\n%s", want, bindings)
		}
	}
	// Nothing in the declaring package but the functions themselves.
	published := readFixtureFile(t, root, "app/web/src/data/skgo_remotes_gen.go")
	for _, want := range []string{
		"Skgo_getThing = getThing",
		"Skgo_doThing = doThing",
		"Skgo_watchThing = watchThing",
		"Skgo_getNamed = getNamed",
	} {
		if !strings.Contains(published, want) {
			t.Errorf("the declaring package does not publish %q:\n%s", want, published)
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

// A `query.batch` is the one kind whose Go signature is not the shape the page
// calls. Go is handed the whole batch and answers all of it; kit's client still
// calls it one argument at a time and gets one value back. So the projection
// has to read the element types out of the two slices — those are what crosses
// the wire per entry — and emit kit's own factory shape:
// `(args: In[]) => (arg: In, idx: number) => Out`.
func TestABatchQueryIsProjectedFromItsElementTypes(t *testing.T) {
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Quote struct {
	Symbol string ` + "`json:\"symbol\"`" + `
}

func getQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	return nil, nil
}

var _ = skgo.BatchQuery(getQuotes)
`, nil)

	if err := Run(cfg); err != nil {
		t.Fatalf("generating an app with a batch query: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.ts")
	want := "export const getQuotes = query.batch('unchecked', (_args: string[]): ((arg: string, idx: number) => Quote) => unimplemented());"
	if !strings.Contains(stub, want) {
		t.Errorf("the stub does not carry %q:\n%s", want, stub)
	}
	// `query.batch` is a property of `query`, so that is the import.
	if !strings.Contains(stub, "import { query } from '$app/server';") {
		t.Errorf("the stub does not import query:\n%s", stub)
	}

	bindings := readFixtureFile(t, root, "app/generated/skgo_bindings_gen.go")
	if want := `skgo.KindBatch,`; !strings.Contains(bindings, want) {
		t.Errorf("the registration file does not carry %q:\n%s", want, bindings)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("`go build ./...` in the generated app: %v\n%s", err, out)
	}
}

// The shapes a batch query is not. Kit's `batch(fn)` with no validator refuses
// every argument, so there is nothing left to batch: a batch always takes one,
// and both sides of it are slices.
func TestABatchQueryThatIsNotABatchIsRefused(t *testing.T) {
	for name, decl := range map[string]string{
		"no argument at all": `func getQuotes(ctx context.Context) ([]string, error) { return nil, nil }`,
		"a single argument":  `func getQuotes(ctx context.Context, symbol string) ([]string, error) { return nil, nil }`,
		"a single result":    `func getQuotes(ctx context.Context, symbols []string) (string, error) { return "", nil }`,
	} {
		t.Run(name, func(t *testing.T) {
			_, cfg := foreignFixture(t, `package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

`+decl+`

var _ = skgo.BatchQuery(getQuotes)
`, nil)

			err := Run(cfg)
			if err == nil {
				t.Fatal("the generator accepted a batch query that cannot be one")
			}
			if !strings.Contains(err.Error(), "A query.batch is func(context.Context, []In) ([]Out, error)") {
				t.Errorf("the refusal does not say what a batch query is: %v", err)
			}
		})
	}
}
