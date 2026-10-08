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

func page(PageRequestEvent) (PageData, error) { return PageData{}, nil }

func save(context.Context) (SaveResult, error) { return SaveResult{}, nil }

func remove(context.Context) error { return nil }

var (
	_ = skgo.Load(page)
	_ = skgo.ActionWithFailure(save, SaveFailure{})
	_ = skgo.ActionNoData(remove)
)
`

func testJavaScriptLoadActions(t *testing.T, root string) {
	stub := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.js")
	for _, want := range []string{
		"/** @typedef {import('./types.js').Item} Item */",
		// The load's return shape is spelled onto `@returns`, not onto an
		// `@type` cast: kit's type writer rewrites an `@type` on an exported
		// function into a `@param` and would leave the body's `never` behind.
		"* @returns {Promise<{ message: string; note?: string; parent: string | null; later: Promise<Item> }>}\n */\nexport const load = async (event) => { throw new Error('skgo: implemented in Go'); };",
		"/** @type {(event: { request: Request }) => Promise<{ saved: boolean } | import('@sveltejs/kit').ActionFailure<{ reason: string }>>} */\n\tsave: async (_event) => { throw new Error('skgo: action implemented in Go'); },",
		"/** @type {(event: { request: Request }) => Promise<void>} */\n\tremove: async (_event) => { throw new Error('skgo: action implemented in Go'); },",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("the JavaScript load/action stub does not carry:\n%s\n---\n%s", want, stub)
		}
	}
	assertThrowingSource(t, stub, true)
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

func testJavaScriptTransportedLoad(t *testing.T, root string) {
	stub := readFixtureFile(t, root, "app/web/src/routes/price/+page.server.js")
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

func testTypeScriptLoadStub(t *testing.T, root string) {
	got := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.ts")
	for _, want := range []string{
		"import type { Item } from './types';",
		"import type { RequestEvent, ActionFailure } from '@sveltejs/kit';",
		"Promise<{ message: string; note?: string; parent: string | null; later: Promise<Item> }>",
		"export const load = async (event: RequestEvent)",
		"throw new Error('skgo: implemented in Go')",
		"Promise<{ saved: boolean } | ActionFailure<{ reason: string }>>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("TypeScript load stub omits %q:\n%s", want, got)
		}
	}
	assertThrowingSource(t, got, false)
	if _, err := os.Stat(filepath.Join(root, "app/web/src/routes/account/+page.server.js")); !os.IsNotExist(err) {
		t.Errorf("a JavaScript counterpart was written in TypeScript mode: %v", err)
	}
}

// Paired with each fixture's literal declaration/type checks: an empty file
// cannot satisfy these output contracts.
func assertThrowingSource(t *testing.T, source string, javascript bool) {
	t.Helper()
	for _, forbidden := range []string{"building", "buildLoad", "prerenderFromGo", "skgoPrerender", "event.platform", "getRequestEvent", "@skgo/sveltekit-adapter/prerender", "inputs:"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("generated source contains build code %q:\n%s", forbidden, source)
		}
	}
	if !javascript && strings.Contains(source, "import(") {
		t.Errorf("generated TypeScript contains inline import type:\n%s", source)
	}
}
