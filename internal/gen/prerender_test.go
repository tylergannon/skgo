package gen

import (
	"go/ast"
	"go/format"
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
	for _, tc := range []struct {
		name string
		in   types.Type
		want string
	}{
		{"build", types.Typ[types.String], `
// inputs_build encodes declared build-time inputs for src/data.remote.ts#build.
func inputs_build(_ context.Context, _ skgo.Call) ([]any, error) {
 values, err := data.SkgoPrerenderInputs_build()
 if err != nil { return nil, err }
 encoded := make([]any, len(values))
 for i, value := range values {
  tree, err := EncodeRoot0(value)
  if err != nil { return nil, fmt.Errorf("skgo: encode prerender input %d: %w", i, err) }
  encoded[i] = tree
 }
 return encoded, nil
}
`},
		{"noArgument", nil, `
// inputs_noArgument encodes declared build-time inputs for src/data.remote.ts#noArgument.
func inputs_noArgument(_ context.Context, _ skgo.Call) ([]any, error) {
 values, err := data.SkgoPrerenderInputs_noArgument()
 if err != nil { return nil, err }
 encoded := make([]any, len(values))
 for i, value := range values { encoded[i] = value }
 return encoded, nil
}
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{}
			fn := &remoteFn{name: tc.name, module: "src/data.remote.ts", goPkg: &goPackage{alias: "data"}, in: tc.in, inCodec: "Root0", inputsHandler: "inputs_" + tc.name}
			var got strings.Builder
			a.writePrerenderInputs(&got, fn)
			// gofmt makes indentation irrelevant without deriving expected bytes from the emitter.
			actual, err := format.Source([]byte("package generated\n" + got.String()))
			if err != nil {
				t.Fatal(err)
			}
			want, err := format.Source([]byte("package generated\n" + tc.want))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != string(want) {
				t.Fatalf("producer:\n%s\nwant:\n%s", actual, want)
			}
		})
	}
}

func TestPrerenderStubSignatures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		language Language
		in       types.Type
		want     string
	}{
		{LanguageTypeScript, types.Typ[types.String], "export const build = prerender('unchecked', async (_arg: string): Promise<string> => unimplemented());\n"},
		{LanguageTypeScript, nil, "export const build = prerender(async (): Promise<string> => unimplemented());\n"},
		{LanguageJavaScript, types.Typ[types.String], "/** @type {import('$app/server').RemotePrerenderFunction<string, string>} */\nexport const build = prerender('unchecked', async (_arg) => unimplemented());\n"},
	} {
		a := &app{cfg: Config{Language: tc.language}}
		got, err := a.stubSignature(&remoteFn{name: "build", kind: kindPrerender, in: tc.in, out: types.Typ[types.String]})
		if err != nil || got != tc.want {
			t.Fatalf("stub=%q, %v; want %q", got, err, tc.want)
		}
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
