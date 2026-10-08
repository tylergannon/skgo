package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const loadSource = `package routes

import "github.com/tylergannon/skgo"

type Data struct { Message string ` + "`json:\"message\"`" + ` }

func site(LayoutRequestEvent) (Data, error) { return Data{Message: "hello"}, nil }

var _ = skgo.Load(site)
`

func TestPrerenderGoLoadFailureNamesAuthoredRoute(t *testing.T) {
	t.Parallel()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(t.TempDir(), "prerender-failure")
	if err := stagePrerenderFixture(root, app, "prerender-failure"); err != nil {
		t.Fatal(err)
	}
	linkExampleFrontendDependencies(t, root, app)

	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("go generate ./...: %v\n%s", err, out)
	}

	cmd := exec.Command(filepath.Join(app, "ui", "node_modules", ".bin", "vp"), "build")
	cmd.Dir = filepath.Join(app, "ui")
	cmd.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	output, buildErr := cmd.CombinedOutput()
	if buildErr == nil {
		t.Fatalf("vp build succeeded despite the Go layout load failure:\n%s", output)
	}
	t.Logf("intentional vp build failure: %v", buildErr)
	got := string(output)
	for _, want := range []string{
		"route ID /about",
		"path /about",
		"source src/routes/layout.server.go",
		"prerender fixture literal failure",
		"GET /about",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("failed frontend build output does not contain %q:\n%s", want, got)
		}
	}
}

func TestPrerenderInputsGenerateGoProducerAndThrowingStubs(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data

import (
	"context"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo"
)

func build(ctx context.Context, name string) (string, error) { return name, nil }
func noArgument(ctx context.Context) (string, error) { return "empty", nil }
func names() ([]string, error) { return []string{"atlas", "beacon"}, nil }
func empty() ([]devalue.UndefinedValue, error) { return []devalue.UndefinedValue{{}}, nil }

var (
	_ = skgo.Prerender(build, skgo.PrerenderOptions{Inputs: names})
	_ = skgo.Prerender(noArgument, skgo.PrerenderOptions{Inputs: empty})
)
`, nil)
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	bindings := readFixtureFile(t, root, "app/generated/skgo_gen.go")
	for _, want := range []string{
		"func inputs_build(_ context.Context, _ skgo.Call) ([]any, error)",
		"values, err := skgo0.SkgoPrerenderInputs_build()",
		"tree, err := EncodeRoot0(value)",
		"Inputs:    inputs_build,",
		"func inputs_noArgument(_ context.Context, _ skgo.Call) ([]any, error)",
		"values, err := skgo0.SkgoPrerenderInputs_noArgument()",
		"encoded[i] = value",
		"Inputs: inputs_noArgument,",
	} {
		if !strings.Contains(bindings, want) {
			t.Errorf("generated bindings omit %q:\n%s", want, bindings)
		}
	}
	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.ts")
	for _, want := range []string{
		"export const build = prerender('unchecked', async (_arg: string): Promise<string> => unimplemented());",
		"export const noArgument = prerender(async (): Promise<string> => unimplemented());",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("generated remote module omits %q:\n%s", want, stub)
		}
	}
	assertThrowingSource(t, stub, false)
	command := readFixtureFile(t, root, "app/generated/prerender/skgo_gen.go")
	if !strings.Contains(command, "generated.Remotes()") {
		t.Fatalf("generated build command omits remote registrations:\n%s", command)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated app does not compile: %v\n%s", err, out)
	}
}

func TestJavaScriptPrerenderStubCarriesArgumentType(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data

import (
	"context"
	"github.com/tylergannon/skgo"
)

func build(context.Context, string) (string, error) { return "", nil }
func inputs() ([]string, error) { return []string{"atlas"}, nil }
var _ = skgo.Prerender(build, skgo.PrerenderOptions{Inputs: inputs})
`, nil)
	cfg.Language = LanguageJavaScript
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.js")
	if !strings.Contains(stub, "@type {import('$app/server').RemotePrerenderFunction<string, string>}") {
		t.Fatalf("generated JavaScript stub lost its concrete input type:\n%s", stub)
	}
	assertThrowingSource(t, stub, true)
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated JavaScript-mode app does not compile: %v\n%s", err, out)
	}
}

func TestPrerenderOptionsFailuresArePositioned(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, expr, want string }{
		{"dynamic false", "skgo.PrerenderOptions{Dynamic:false}", "Dynamic is not supported"},
		{"validate nil", "skgo.PrerenderOptions{Validate:nil}", "Validate is not supported"},
		{"unknown", "skgo.PrerenderOptions{Inputs:producer,Other:true}", `unknown PrerenderOptions field "Other"`},
		{"variable", "options", "must be a keyed"},
		{"anonymous", "skgo.PrerenderOptions{Inputs:func()([]string,error){return nil,nil}}", "must name a local function"},
		{"duplicate", "skgo.PrerenderOptions{Inputs:first,Inputs:second}", "duplicate PrerenderOptions field"},
		{"unkeyed", "skgo.PrerenderOptions{producer}", "must use keyed syntax"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			expr, err := parser.ParseExprFrom(fset, "data.remote.go", tc.expr, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
			ast.Inspect(expr, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && id.Name == "skgo" {
					info.Uses[id] = types.NewPkgName(id.Pos(), nil, "skgo", types.NewPackage(skgoPkg, "skgo"))
				}
				return true
			})
			p := &packages.Package{Fset: fset, TypesInfo: info}
			_, err = (&app{}).readPrerenderOptions(p, expr)
			if err == nil || !strings.Contains(err.Error(), "data.remote.go:1:") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("missing positioned %q: %v", tc.want, err)
			}
		})
	}
}

func TestPrerenderInputProducerMustMatchBodyArgument(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		source string
		valid  bool
	}{
		{"func producer()([]string,error){return nil,nil}", true},
		{"func producer()([]int,error){return nil,nil}", false},
		{"func producer() (string,error){return \"\",nil}", false},
		{"func producer(string)([]string,error){return nil,nil}", false},
		{"func producer()([]string,int){return nil,0}", false},
	} {
		fn := declarationFunction(t, "package data\n"+tc.source, "producer")
		err := prerenderInputsSignature(fn, types.Typ[types.String])
		if tc.valid {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "matching the published argument") {
			t.Fatalf("lost producer restriction: %v", err)
		}
	}
}

func TestNoArgumentPrerenderInputsCompileWithoutFmtImport(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data

import (
	"context"
	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo"
)

func build(context.Context) (string, error) { return "empty", nil }
func inputs() ([]devalue.UndefinedValue, error) { return []devalue.UndefinedValue{{}}, nil }
var _ = skgo.Prerender(build, skgo.PrerenderOptions{Inputs: inputs})
`, nil)
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Codecs share the output file now and legitimately use fmt. The build
	// catches any unused imports left by a no-argument producer.
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated no-argument app does not compile: %v\n%s", err, out)
	}
}

func TestPrerenderInputHandlersHaveUniqueNamesAcrossModules(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data

import (
	"context"
	"github.com/tylergannon/skgo"
)

func item(context.Context, string) (string, error) { return "data", nil }
func inputs() ([]string, error) { return []string{"data"}, nil }
var _ = skgo.Prerender(item, skgo.PrerenderOptions{Inputs: inputs})
`, map[string]string{
		"app/web/src/other/other.remote.go": `package other

import (
	"context"
	"github.com/tylergannon/skgo"
)

func item(context.Context, string) (string, error) { return "other", nil }
func inputs() ([]string, error) { return []string{"other"}, nil }
var _ = skgo.Prerender(item, skgo.PrerenderOptions{Inputs: inputs})
`,
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	bindings := readFixtureFile(t, root, "app/generated/skgo_gen.go")
	for _, want := range []string{"func inputs_item(", "func inputs_item_2(", "Inputs:    inputs_item,", "Inputs:    inputs_item_2,"} {
		if !strings.Contains(bindings, want) {
			t.Errorf("generated bindings omit %q:\n%s", want, bindings)
		}
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated multi-module app does not compile: %v\n%s", err, out)
	}
}

func TestTransportedPrerenderInputsUseRuntimeTransportEncoding(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data

import (
	"context"
	hooks "example.com/app/web/src"
	"github.com/tylergannon/skgo"
)

func build(context.Context, hooks.Money) (string, error) { return "money", nil }
func inputs() ([]hooks.Money, error) { return []hooks.Money{{Cents: 125}}, nil }
var _ = skgo.Prerender(build, skgo.PrerenderOptions{Inputs: inputs})
`, map[string]string{
		"app/web/src/hooks.go": `package hooks

import "github.com/tylergannon/skgo"

type Money struct { Cents int ` + "`json:\"cents\"`" + ` }
var _ = skgo.Transported[Money]("Money")
`,
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	bindings := readFixtureFile(t, root, "app/generated/skgo_gen.go")
	if !strings.Contains(bindings, "tree, err := call.Transported(value)") {
		t.Fatalf("transported input does not reach the app's native transport encoder:\n%s", bindings)
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = filepath.Join(root, "app")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated transported-input app does not compile: %v\n%s", err, out)
	}
}

func linkExampleFrontendDependencies(t *testing.T, root, app string) {
	t.Helper()
	linkFrontendDependencies(t, root, filepath.Join(app, "ui", "node_modules"))
}

func linkFrontendDependencies(t *testing.T, root, destination string) {
	t.Helper()
	if err := frontendDependencies(root, destination); err != nil {
		t.Fatal(err)
	}
}

// Each build owns Vite's mutable directories; only pinned dependencies are shared.
func frontendDependencies(root, destination string) error {
	source := filepath.Join(root, "example/web/node_modules")
	if _, err := os.Stat(filepath.Join(source, ".bin/vp")); err != nil {
		return fmt.Errorf("pinned frontend dependencies missing: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(destination, "@skgo"), 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "@skgo" || entry.Name() == "$app" || entry.Name() == ".vite-temp" {
			continue
		}
		if err := os.Symlink(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return os.Symlink(filepath.Join(root, "internal/adapter"), filepath.Join(destination, "@skgo/sveltekit-adapter"))
}

func TestAPrerenderedPageCanHaveAGoLayoutLoadInItsBranch(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/layout.server.go":   loadSource,
		"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":     "export const prerender = true;\n",
	})

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if command := readFixtureFile(t, root, "app/generated/prerender/skgo_gen.go"); !strings.Contains(command, "nil, generated.Loads()") {
		t.Fatalf("build command omits generated loads:\n%s", command)
	}
}

// Kit 3 PageNodes reduces universal ?? server ?? inherited, and its page
// handler rejects actions on a true/auto branch. Exercise that decision with
// actual files and an action; generation success alone never reached it.
func TestPrerenderInheritanceMatchesKit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, layout, server, page string
		reject                     bool
	}{
		{"inherited true", "true", "", "", true},
		{"leaf false", "true", "", "false", false},
		{"leaf true", "false", "", "true", true},
		{"universal wins", "false", "true", "", false},
		{"auto", "", "", "'auto'", true},
		{"unknown suppresses server", "choice", "true", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			web := t.TempDir()
			root := filepath.Join(web, "src/routes")
			page := filepath.Join(root, "about")
			writeSharedFixture(t, web, "src/routes/about/+page.svelte", "<p>fixture</p>")
			if tc.layout != "" {
				writeSharedFixture(t, web, "src/routes/+layout.ts", "const choice=false; export const prerender = "+tc.layout+";\n")
			}
			if tc.server != "" {
				writeSharedFixture(t, web, "src/routes/+layout.server.ts", "export const prerender = "+tc.server+";\n")
			}
			if tc.page != "" {
				writeSharedFixture(t, web, "src/routes/about/+page.ts", "export const prerender = "+tc.page+";\n")
			}
			a := &app{cfg: Config{Web: web}, actions: []*actionFn{{stub: filepath.Join(page, "+page.server.ts"), pos: token.Position{Filename: "page.server.go", Line: 7}}}}
			err := a.checkPrerenderedActions()
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "cannot prerender page /about with actions at page.server.go:7") {
					t.Fatalf("lost branch rejection: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadsOutsideThePrerenderedBranchAreAccepted(t *testing.T) {
	t.Parallel()
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

func TestAPrerenderedPageCanHaveItsOwnGoLoad(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/about/page.server.go": strings.ReplaceAll(loadSource, "LayoutRequestEvent", "PageRequestEvent"),
		"app/web/src/routes/about/+page.svelte":   "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":       "export const prerender = true;\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestANamedLayoutResetExcludesLoadsOutsideKitsBranch(t *testing.T) {
	t.Parallel()
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

func TestTheLoadStubContainsOnlyTypedThrowingBody(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/account/page.server.go": strings.ReplaceAll(loadSource, "LayoutRequestEvent", "PageRequestEvent"),
		"app/web/src/routes/account/+page.svelte":   "<h1>Account</h1>\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stub := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.ts")
	for _, want := range []string{
		"import type { RequestEvent } from '@sveltejs/kit';",
		"export const load = async (event: RequestEvent): Promise<{ message: string }>",
		"throw new Error('skgo: implemented in Go')",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("stub does not contain %q:\n%s", want, stub)
		}
	}
	assertThrowingSource(t, stub, false)
}

func TestOptionExamplesInCommentsAndStringsAreIgnored(t *testing.T) {
	t.Parallel()
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
