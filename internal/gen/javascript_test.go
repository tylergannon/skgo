package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsFixtureRemote exercises every remote kind and the named wire types a
// JavaScript consumer has to see. `Status` carries an optional and a nullable
// field, which are the two field constructors whose JSDoc shape differs from a
// required property.
const jsFixtureRemote = `package data

import (
	"context"

	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo"
 "example.com/app/generated/params"
)

type Status struct {
	Message string                    ` + "`json:\"message\"`" + `
	Writes  int                       ` + "`json:\"writes\"`" + `
	Note    polytype.Optional[string] ` + "`json:\"note,omitzero\"`" + `
	Parent  polytype.Nullable[string] ` + "`json:\"parent\"`" + `
}

type FormInput struct {
	Text  string                 ` + "`json:\"text\"`" + `
	Count polytype.Optional[int] ` + "`json:\"count,omitzero\"`" + `
}

type Item struct {
	Text string ` + "`json:\"text\"`" + `
}

func getThing(ctx context.Context) (Item, error) { return Item{}, nil }

func getStatusFor(ctx context.Context, name string) (Status, error) { return Status{}, nil }

func record(event params.RequestEvent, name string) (Status, error) { return Status{}, nil }

func doThing(event params.RequestEvent) (Item, error) { return Item{}, nil }

func watchStatus(ctx context.Context, name string, yield func(Status) error) error { return nil }

func watchAll(ctx context.Context, yield func(Item) error) error { return nil }

func getQuotes(ctx context.Context, names []string) ([]Item, error) { return nil, nil }

func submitItem(event params.RequestEvent, in FormInput) (Status, error) { return Status{}, nil }

var (
	_ = skgo.Query(getThing)
	_ = skgo.Query(getStatusFor)
	_ = skgo.Command(record)
	_ = skgo.Command(doThing)
	_ = skgo.LiveQuery(watchStatus)
	_ = skgo.LiveQuery(watchAll)
	_ = skgo.BatchQuery(getQuotes)
	_ = skgo.Form(submitItem)
)
`

// TestJavaScriptModeEmitsJSDocRemotesAndTypes is the contract for the
// JavaScript projection: one `.remote.js` per remote module with a JSDoc @type
// on every exported declaration, and one `types.js` from polytype's own
// JavaScript backend with the named wire types. The TypeScript counterparts do
// not exist, and the module carries no runtime type binding for kit to trip
// over.
//
// The JSDoc shapes above are not invented here: a consumer over this same
// output was type-checked with svelte-check against the pinned Kit and
// TypeScript 6, passing with no diagnostics, and rejecting a wrong argument
// type (`record(123)`), a wrong result property (`status.nope`) and a nullable
// value used as non-null. Every generic in the @type annotations mirrors kit's
// own declarations for `query`, `query.live`, `query.batch`, `command` and
// `form`.
func TestJavaScriptModeEmitsJSDocRemotesAndTypes(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, jsFixtureRemote, nil)
	cfg.Language = LanguageJavaScript

	if err := Run(cfg); err != nil {
		t.Fatalf("generating a JavaScript app: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.js")
	for _, want := range []string{
		"import { command, form, query } from '$app/server';",
		"/** @typedef {import('./types.js').FormInput} FormInput */",
		"/** @typedef {import('./types.js').Item} Item */",
		"/** @typedef {import('./types.js').Status} Status */",
		"const unimplemented = () => {\n\tthrow new Error('skgo: implemented in Go');\n};",
		// query, no argument and with one.
		"/** @type {import('$app/server').RemoteQueryFunction<void, Item>} */\nexport const getThing = query(() => unimplemented());",
		"/** @type {import('$app/server').RemoteQueryFunction<string, Status>} */\nexport const getStatusFor = query('unchecked', (_arg) => unimplemented());",
		// command, no argument and with one.
		"/** @type {import('$app/server').RemoteCommand<void, Item>} */\nexport const doThing = command(() => unimplemented());",
		"/** @type {import('$app/server').RemoteCommand<string, Status>} */\nexport const record = command('unchecked', (_arg) => unimplemented());",
		// live, no argument and with one.
		"/** @type {import('$app/server').RemoteLiveQueryFunction<void, Item>} */\nexport const watchAll = query.live(() => unimplemented());",
		"/** @type {import('$app/server').RemoteLiveQueryFunction<string, Status>} */\nexport const watchStatus = query.live('unchecked', (_arg) => unimplemented());",
		// batch stays the query function kit's batch produces.
		"/** @type {import('$app/server').RemoteQueryFunction<string, Item>} */\nexport const getQuotes = query.batch('unchecked', (_args) => unimplemented());",
		// form names kit's form type.
		"/** @type {import('$app/server').RemoteForm<FormInput, Status>} */\nexport const submitItem = form('unchecked', (_arg) => unimplemented());",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("the JavaScript stub does not carry:\n%s\n---\n%s", want, stub)
		}
	}
	// Kit reads the module's runtime exports and refuses anything that is not a
	// remote function, so a type must never be a runtime export: no `export
	// type`, no `import type`, and every actual export is a remote function.
	for _, absent := range []string{"export type", "import type"} {
		if strings.Contains(stub, absent) {
			t.Errorf("the JavaScript stub carries %q, which is not valid JavaScript or would add a runtime export:\n%s", absent, stub)
		}
	}
	for _, line := range strings.Split(stub, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export ") && !strings.HasPrefix(line, "export const ") {
			t.Errorf("the JavaScript stub has a non-function runtime export: %q\n%s", line, stub)
		}
	}

	types := readFixtureFile(t, root, "app/web/src/data/types.js")
	for _, want := range []string{
		"@typedef {Object} Status",
		"@property {string} message",
		"@property {number} writes",
		"@property {string} [note]",
		"@property {string | null} parent",
		"@typedef {Object} FormInput",
		"@property {string} text",
		"@property {number} [count]",
		"export {};",
	} {
		if !strings.Contains(types, want) {
			t.Errorf("types.js does not carry:\n%s\n---\n%s", want, types)
		}
	}

	// No `.ts` counterparts: the generator wrote JavaScript for this app.
	for _, gone := range []string{"app/web/src/data/data.remote.ts", "app/web/src/data/types.ts"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("%s still exists in JavaScript mode: %v", gone, err)
		}
	}
}

// TestJavaScriptModeIsChosenFromTheApp: the language choice has to survive a
// regenerate, and `go generate` passes no flag, so the generator reads it off
// the app sv created. sv writes a JavaScript app a jsconfig.json.
func TestJavaScriptModeIsChosenFromTheApp(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, jsFixtureRemote, map[string]string{
		"app/web/jsconfig.json": "{\n  \"extends\": \"$app/tsconfig\"\n}\n",
	})

	if err := Run(cfg); err != nil {
		t.Fatalf("generating an app whose own config is JavaScript: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "app", "web", "src", "data", "data.remote.js")); err != nil {
		t.Fatalf("a jsconfig.json app was not generated as JavaScript: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "web", "src", "data", "data.remote.ts")); !os.IsNotExist(err) {
		t.Fatalf("a jsconfig.json app still has a TypeScript stub: %v", err)
	}
}

// TestSwitchingLanguageRemovesOnlyGeneratedFiles: regenerating after a choice
// changes has to clear the mode that was replaced. The old modules are
// generated artifacts; an authored file beside them must survive.
func TestSwitchingLanguageRemovesOnlyGeneratedFiles(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, jsFixtureRemote, nil)

	cfg.Language = LanguageTypeScript
	if err := Run(cfg); err != nil {
		t.Fatalf("generating TypeScript first: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "web", "src", "data", "data.remote.ts")); err != nil {
		t.Fatalf("TypeScript stub was not written: %v", err)
	}

	// An authored module beside the generated one is not the generator's to
	// remove.
	authored := filepath.Join(root, "app", "web", "src", "data", "handwritten.js")
	if err := os.WriteFile(authored, []byte("export const mine = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg.Language = LanguageJavaScript
	if err := Run(cfg); err != nil {
		t.Fatalf("regenerating as JavaScript: %v", err)
	}
	for _, gone := range []string{"app/web/src/data/data.remote.ts", "app/web/src/data/types.ts"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("%s was not removed when the language changed: %v", gone, err)
		}
	}
	if _, err := os.Stat(authored); err != nil {
		t.Errorf("switching languages removed an authored file: %v", err)
	}
}

// TestJavaScriptModeKeepsTheCollisionRefusal: polytype's identifier allocation
// can rename a root, and the JavaScript stubs refer to types by name exactly as
// the TypeScript ones do, so the refusal has to hold in both modes.
func TestJavaScriptModeKeepsTheCollisionRefusal(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, `package data

import (
	"context"

	"example.com/wire"
	"github.com/tylergannon/skgo"
)

// Thing wraps the dependency's Thing, under the same name.
type Thing struct {
	Inner wire.Thing `+"`json:\"inner\"`"+`
}

func getThing(ctx context.Context) (Thing, error) {
	return Thing{}, nil
}

var _ = skgo.Query(getThing)
`, nil)
	cfg.Language = LanguageJavaScript

	err := Run(cfg)
	if err == nil {
		t.Fatal("a JavaScript app whose Thing reaches a second Thing was projected")
	}
	if !strings.Contains(err.Error(), "Thing") {
		t.Fatalf("the refusal does not name the type:\n%v", err)
	}
}
