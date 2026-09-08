package gen

import (
	"os"
	"strings"
	"testing"
)

const loadSource = `package routes

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Data struct { Message string ` + "`json:\"message\"`" + ` }

func site(context.Context) (Data, error) { return Data{Message: "hello"}, nil }

var _ = skgo.Load(site)
`

func TestAPrerenderedPageCannotHaveAGoLayoutLoadInItsBranch(t *testing.T) {
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/layout.server.go":   loadSource,
		"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":     "export const prerender = true;\n",
	})

	err := Run(cfg)
	want := "skgo: route /about is prerendered, and its branch has a Go server load at src/routes/layout.server.go; skgo cannot answer a load while kit prerenders (#81). Remove the prerender or move the load"
	if err == nil || err.Error() != want {
		t.Fatalf("Run error = %v, want exactly:\n%s", err, want)
	}
}

func TestPrerenderInheritanceMatchesKit(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		fails bool
	}{
		{
			name: "a leaf inherits true from its layout",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/+layout.ts":         "export const prerender = true;\n",
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
			},
			fails: true,
		},
		{
			name: "a leaf false overrides a layout true",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/+layout.ts":         "export const prerender = true;\n",
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
				"app/web/src/routes/about/+page.ts":     "export const prerender = false;\n",
			},
		},
		{
			name: "a leaf true overrides a layout false",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/+layout.ts":         "export const prerender = false;\n",
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
				"app/web/src/routes/about/+page.ts":     "export const prerender = true;\n",
			},
			fails: true,
		},
		{
			name: "a universal option wins over the same node server option",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":         loadSource,
				"app/web/src/routes/branch/+layout.ts":        "export const prerender = false;\n",
				"app/web/src/routes/branch/+layout.server.ts": "export const prerender = true;\n",
				"app/web/src/routes/branch/page/+page.svelte": "<h1>Page</h1>\n",
			},
		},
		{
			name: "auto can enter kits prerender crawl",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
				"app/web/src/routes/about/+page.ts":     "export const prerender: boolean | 'auto' = 'auto';\n",
			},
			fails: true,
		},
		{
			name: "an unknown universal option does not fall back to the server option",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":         loadSource,
				"app/web/src/routes/branch/+layout.ts":        "const choice = false;\nexport const prerender = choice;\n",
				"app/web/src/routes/branch/+layout.server.ts": "export const prerender = true;\n",
				"app/web/src/routes/branch/page/+page.svelte": "<h1>Page</h1>\n",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, cfg := foreignFixture(t, "", test.files)
			err := Run(cfg)
			if test.fails && (err == nil || !strings.Contains(err.Error(), "cannot answer a load while kit prerenders (#81)")) {
				t.Fatalf("Run error = %v, want prerender refusal", err)
			}
			if !test.fails && err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	}
}

func TestLoadsOutsideThePrerenderedBranchAreAccepted(t *testing.T) {
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/private/layout.server.go": loadSource,
		"app/web/src/routes/private/+page.svelte":     "<h1>Private</h1>\n",
		"app/web/src/routes/about/+page.svelte":       "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":           "export const prerender = true;\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPrerenderedPageCannotHaveItsOwnGoLoad(t *testing.T) {
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/about/page.server.go": loadSource,
		"app/web/src/routes/about/+page.svelte":   "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":       "export const prerender = true;\n",
	})
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "src/routes/about/page.server.go") {
		t.Fatalf("Run error = %v, want refusal naming the page load", err)
	}
}

func TestANamedLayoutResetExcludesLoadsOutsideKitsBranch(t *testing.T) {
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/+layout.svelte":            "<slot />\n",
		"app/web/src/routes/+layout.ts":                "export const prerender = true;\n",
		"app/web/src/routes/branch/+layout.svelte":     "<slot />\n",
		"app/web/src/routes/branch/layout.server.go":   loadSource,
		"app/web/src/routes/branch/page/+page@.svelte": "<h1>Page</h1>\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestTheLoadStubExplainsAPrerenderCall(t *testing.T) {
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/account/page.server.go": loadSource,
		"app/web/src/routes/account/+page.svelte":   "<h1>Account</h1>\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stub := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.ts")
	for _, want := range []string{
		"import { building } from '$app/env'",
		"event.url.pathname",
		"src/routes/account/page.server.go",
		"skgo cannot answer a load while kit prerenders (#81)",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("stub does not contain %q:\n%s", want, stub)
		}
	}
}

func TestOptionExamplesInCommentsAndStringsAreIgnored(t *testing.T) {
	for _, source := range []string{
		"// export const prerender = true;\nexport const nope = false;\n",
		"const example = 'export const prerender = true';\n",
		"/* export const prerender = 'auto'; */\n",
	} {
		if value, ok, err := sourcePrerender(writeOptionFixture(t, source)); err != nil || ok {
			t.Fatalf("sourcePrerender = %q, %v, %v; want no option", value, ok, err)
		}
	}
}

func writeOptionFixture(t *testing.T, source string) string {
	t.Helper()
	path := t.TempDir() + "/+page.ts"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
