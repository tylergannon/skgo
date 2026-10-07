package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// javaScriptLoadActionFixture carries one page's Go server load and both an
// action with a typed failure and one with no data. Its result has the four
// field shapes a page's JSDoc has to preserve: a required value, an optional
// one, a nullable one, and a deferred Promise<T>.
const javaScriptLoadActionFixture = `package routes

import (
	"context"

	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo"
)

type Item struct {
	Text string ` + "`json:\"text\"`" + `
}

type PageData struct {
	Message string                    ` + "`json:\"message\"`" + `
	Note    polytype.Optional[string] ` + "`json:\"note,omitzero\"`" + `
	Parent  polytype.Nullable[string] ` + "`json:\"parent\"`" + `
	Later   skgo.Deferred[Item]       ` + "`json:\"later\"`" + `
}

type SaveResult struct {
	Saved bool ` + "`json:\"saved\"`" + `
}

type SaveFailure struct {
	Reason string ` + "`json:\"reason\"`" + `
}

func page(RequestEvent) (PageData, error) { return PageData{}, nil }

func save(context.Context) (SaveResult, error) { return SaveResult{}, nil }

func remove(context.Context) error { return nil }

var (
	_ = skgo.Load(page)
	_ = skgo.ActionWithFailure(save, SaveFailure{})
	_ = skgo.ActionNoData(remove)
)
`

// TestJavaScriptModeEmitsJSDocServerLoadsAndActions is the contract for the
// JavaScript mode's `+page.server.js`: a load whose JSDoc carries the whole
// object kit's type writer reads back out, and actions whose JSDoc carries
// their success and `ActionFailure` return unions. The TypeScript counterpart
// is not written.
func TestJavaScriptModeEmitsJSDocServerLoadsAndActions(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/account/page.server.go": javaScriptLoadActionFixture,
	})
	cfg.Language = LanguageJavaScript

	if err := Run(cfg); err != nil {
		t.Fatalf("generating a JavaScript app: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.js")
	for _, want := range []string{
		"import { building } from '$app/env';",
		"/** @typedef {import('./types.js').Item} Item */",
		// The load's return shape is spelled onto `@returns`, not onto an
		// `@type` cast: kit's type writer rewrites an `@type` on an exported
		// function into a `@param` and would leave the body's `never` behind.
		"/**\n * @param {string} route\n * @returns {never}\n */\nconst unimplemented = (route) => {",
		"* @returns {Promise<{ message: string; note?: string; parent: string | null; later: Promise<Item> }>}\n */\nexport const load = async (event) => building ? buildLoad(",
		"/** @type {(event: { request: Request }) => Promise<{ saved: boolean } | import('@sveltejs/kit').ActionFailure<{ reason: string }>>} */\n\tsave: async (_event) => { throw new Error('skgo: action implemented in Go'); },",
		"/** @type {(event: { request: Request }) => Promise<void>} */\n\tremove: async (_event) => { throw new Error('skgo: action implemented in Go'); },",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("the JavaScript load/action stub does not carry:\n%s\n---\n%s", want, stub)
		}
	}
	if strings.Contains(stub, "import type") || strings.Contains(stub, "export type") {
		t.Errorf("the JavaScript stub carries a TypeScript-only declaration:\n%s", stub)
	}

	if _, err := os.Stat(filepath.Join(root, "app/web/src/routes/account/+page.server.ts")); !os.IsNotExist(err) {
		t.Errorf("a TypeScript counterpart was written in JavaScript mode: %v", err)
	}

	list := readFixtureFile(t, root, "app/web/skgo.remotes.json")
	for _, want := range []string{
		`"src/routes/account/+page.server.js"`,
	} {
		if !strings.Contains(list, want) {
			t.Errorf("skgo.remotes.json does not carry %s:\n%s", want, list)
		}
	}
	if strings.Contains(list, `+page.server.ts`) {
		t.Errorf("skgo.remotes.json names a TypeScript module in JavaScript mode:\n%s", list)
	}
}

func TestJavaScriptModeAcceptsAPrerenderedLoad(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/about/page.server.go": loadSource,
		"app/web/src/routes/about/+page.ts":       "export const prerender = true;\n",
	})
	cfg.Language = LanguageJavaScript

	if err := Run(cfg); err != nil {
		t.Fatalf("generating a JavaScript app's prerendered Go load: %v", err)
	}
}

// TestJavaScriptModeLoadImportsATransportedClassFromHooks: a load's result may
// carry one of the app's own classes, which reaches the browser through kit's
// `transport` hook. In JavaScript the class cannot be an `import type`, so the
// stub names it with a top-level JSDoc typedef pointing at `src/hooks.js`,
// and the reference keeps the class's methods in the page's type.
func TestJavaScriptModeLoadImportsATransportedClassFromHooks(t *testing.T) {
	t.Parallel()
	const load = `package routes

import (
	hooks "example.com/app/web/src"
	"github.com/tylergannon/skgo"
)

type PageData struct {
	Price hooks.Money ` + "`json:\"price\"`" + `
}

func page(RequestEvent) (PageData, error) { return PageData{}, nil }

var _ = skgo.Load(page)
`
	const hooks = `package hooks

import "github.com/tylergannon/skgo"

type Money struct {
	Cents int ` + "`json:\"cents\"`" + `
}

var _ = skgo.Transported[Money]("Money")
`
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/hooks.go":                      hooks,
		"app/web/src/routes/account/page.server.go": load,
	})
	cfg.Language = LanguageJavaScript

	if err := Run(cfg); err != nil {
		t.Fatalf("generating a JavaScript app with a transported load: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.js")
	if command := readFixtureFile(t, root, "app/generated/prerender/skgo_gen.go"); !strings.Contains(command, "generated.Transport(), generated.Loads()") {
		t.Fatalf("a transported load lost its build-time transport: %s", command)
	}
	for _, want := range []string{
		"/** @typedef {import('../../hooks.js').Money} Money */",
		"* @returns {Promise<{ price: Money }>}",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("the JavaScript load stub does not carry:\n%s\n---\n%s", want, stub)
		}
	}
}

// TestTypeScriptModeLoadStubCarriesTheBuildBridge checks that both modes
// preserve their declared load data shape while Kit can call the Go load.
func TestTypeScriptModeLoadStubCarriesTheBuildBridge(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/account/page.server.go": javaScriptLoadActionFixture,
	})
	cfg.Language = LanguageTypeScript

	if err := Run(cfg); err != nil {
		t.Fatalf("generating a TypeScript app: %v", err)
	}

	got := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.ts")
	for _, want := range []string{
		"import type { Item } from './types';",
		"skgoPrerenderLoad",
		"Promise<{ message: string; note?: string; parent: string | null; later: Promise<Item> }>",
		"building ? buildLoad(\"src/routes/account/+page.server.ts\", \"src/routes/account/page.server.go\", event)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("TypeScript load stub omits %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "app/web/src/routes/account/+page.server.js")); !os.IsNotExist(err) {
		t.Errorf("a JavaScript counterpart was written in TypeScript mode: %v", err)
	}
}
