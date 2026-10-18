package gen

import (
	"go/types"
	"strings"
	"testing"
)

// The factory arity and Go argument admission must agree for each owned kind.
func TestRemoteArityEmitters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                  string
		kind                  remoteKind
		in, out               types.Type
		codec, stub, argument string
	}{
		{"getThing", kindQuery, nil, types.Typ[types.String], "",
			"export const getThing = query((): string => unimplemented());\n",
			"\tif err := skgo.RefuseArgument(call); err != nil {\n\t\treturn nil, err\n\t}\n"},
		{"doThing", kindCommand, nil, types.Typ[types.String], "",
			"export const doThing = command((): string => unimplemented());\n",
			"\tif err := skgo.RefuseArgument(call); err != nil {\n\t\treturn nil, err\n\t}\n"},
		{"watchThing", kindLive, nil, types.Typ[types.Int], "",
			"export const watchThing = query.live((): AsyncIterable<number> => unimplemented());\n",
			"\tif err := skgo.RefuseArgument(call); err != nil {\n\t\treturn nil, err\n\t}\n"},
		{"getNamed", kindQuery, types.Typ[types.String], types.Typ[types.String], "Root0",
			"export const getNamed = query('unchecked', (_arg: string): string => unimplemented());\n",
			"\tif err := skgo.RequireArgument(call); err != nil {\n\t\treturn nil, err\n\t}\n\tin, err := DecodeRoot0(call.Arg)\n\tif err != nil {\n\t\treturn nil, call.RejectArgument(err)\n\t}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{}
			fn := &remoteFn{name: tc.name, kind: tc.kind, in: tc.in, out: tc.out, inCodec: tc.codec}
			stub, err := a.stubSignature(fn)
			if err != nil || stub != tc.stub {
				t.Fatalf("stub = %q, %v; want %q", stub, err, tc.stub)
			}
			var argument strings.Builder
			a.writeArgument(&argument, fn, "\t", "return nil, err")
			if got := argument.String(); got != tc.argument {
				t.Fatalf("argument = %q; want %q", got, tc.argument)
			}
		})
	}
}

// The marker takes `any`, so it is `skgo generate` that has to report a
// function it cannot publish — at the declaration's own position, saying what
// the shape should have been. A message that only says "cannot read the types"
// leaves the developer to guess.
func TestRemoteSignaturesRejectUnsupportedFunctions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		decl    string
		wantAll []string
	}{
		{
			name: "no context at all",
			decl: `func getThing(id string) (string, error) { return "", nil }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"query", "func(context.Context) (Out, error)"},
		},
		{
			name: "no error result",
			decl: `func getThing(ctx context.Context) string { return "" }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"query", "(Out, error)"},
		},
		{
			name: "two arguments",
			decl: `func getThing(ctx context.Context, a string, b string) (string, error) { return "", nil }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"query"},
		},
		{
			name: "variadic",
			decl: `func getThing(ctx context.Context, ids ...string) (string, error) { return "", nil }

var _ = skgo.Query(getThing)`,
			wantAll: []string{"variadic"},
		},
		{
			name: "a live query with no yield",
			decl: `func watchThing(ctx context.Context) error { return nil }

var _ = skgo.LiveQuery(watchThing)`,
			wantAll: []string{"query.live", "func(Out) error"},
		},
		{
			name: "a live query that returns a value",
			decl: `func watchThing(ctx context.Context, yield func(int) error) (int, error) { return 0, nil }

var _ = skgo.LiveQuery(watchThing)`,
			wantAll: []string{"query.live"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, kind := "getThing", kindQuery
			if strings.Contains(tc.decl, "watchThing") {
				name, kind = "watchThing", kindLive
			}
			declaration := strings.Split(tc.decl, "var _ =")[0]
			fn := declarationFunction(t, "package data\nimport \"context\"\nvar _ = context.Background\n"+declaration, name)
			_, _, err := remoteSignature(kind, fn)
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

func TestABatchQueryIsProjectedFromItsElementTypes(t *testing.T) {
	t.Parallel()
	fn := declarationFunction(t, `package data
import "context"
type Quote struct { Symbol string }
func getQuotes(context.Context, []string) ([]Quote,error) {return nil,nil}
`, "getQuotes")
	in, out, err := remoteSignature(kindBatch, fn)
	if err != nil {
		t.Fatal(err)
	}
	stub, err := (&app{}).stubSignature(&remoteFn{name: "getQuotes", kind: kindBatch, in: in, out: out})
	const want = "export const getQuotes = query.batch('unchecked', (_args: string[]): ((arg: string, idx: number) => Quote) => unimplemented());\n"
	if err != nil || stub != want {
		t.Fatalf("stub = %q, %v; want %q", stub, err, want)
	}
}

// The shapes a batch query is not. Kit's `batch(fn)` with no validator refuses
// every argument, so there is nothing left to batch: a batch always takes one,
// and both sides of it are slices.
func TestABatchQueryThatIsNotABatchIsRefused(t *testing.T) {
	t.Parallel()
	for name, decl := range map[string]string{
		"no argument at all": `func getQuotes(ctx context.Context) ([]string, error) { return nil, nil }`,
		"a single argument":  `func getQuotes(ctx context.Context, symbol string) ([]string, error) { return nil, nil }`,
		"a single result":    `func getQuotes(ctx context.Context, symbols []string) (string, error) { return "", nil }`,
	} {
		t.Run(name, func(t *testing.T) {
			fn := declarationFunction(t, "package data\nimport \"context\"\n"+decl, "getQuotes")
			_, _, err := remoteSignature(kindBatch, fn)
			if err == nil {
				t.Fatal("the generator accepted a batch query that cannot be one")
			}
			if !strings.Contains(err.Error(), "A query.batch is func(context.Context, []In) ([]Out, error)") {
				t.Errorf("the refusal does not say what a batch query is: %v", err)
			}
		})
	}
}
