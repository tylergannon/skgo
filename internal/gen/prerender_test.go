package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
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
