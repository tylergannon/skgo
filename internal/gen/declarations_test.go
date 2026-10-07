package gen

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Type-check only the declaration under test. Dependencies supplied here are
// ordinary Go type graphs; no module, generator, subprocess or route tree is
// needed to distinguish signature and wire rules.
var declarationImportMu sync.Mutex
var declarationStandardImporter = importer.Default()

type declarationImports map[string]*types.Package

func (i declarationImports) Import(path string) (*types.Package, error) {
	if p := i[path]; p != nil {
		return p, nil
	}
	declarationImportMu.Lock()
	defer declarationImportMu.Unlock()
	return declarationStandardImporter.Import(path)
}
func declarationPackage(t *testing.T, path, filename, source string, imports declarationImports) *packages.Package {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Instances: map[*ast.Ident]types.Instance{}}
	pkg, err := (&types.Config{Importer: imports}).Check(path, fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	return &packages.Package{Types: pkg, TypesInfo: info, Fset: fset, Syntax: []*ast.File{file}}
}
func declarationFunction(t *testing.T, source, name string) *types.Func {
	t.Helper()
	pkg := declarationPackage(t, "fixture", "data.remote.go", source, nil)
	fn, ok := pkg.Types.Scope().Lookup(name).(*types.Func)
	if !ok {
		t.Fatalf("missing function %s", name)
	}
	return fn
}

// These stand-ins expose the real named/generic identities readMarker cares
// about. Signature rules need Go's type checker, not a whole generated app.
func markerDeclaration(t *testing.T, filename, source string) (*app, *goPackage, *packages.Package, *ast.CallExpr) {
	t.Helper()
	imports := declarationImports{}
	imports[skgoPkg] = declarationPackage(t, skgoPkg, "skgo.go", `package skgo
import("context";"net/http")
type RequestResolve[P,L any] func(context.Context,RequestEvent[P,L])(*http.Response,error)
type RequestMiddleware[P,L any] func(context.Context,RequestEvent[P,L],RequestResolve[P,L])(*http.Response,error)
type RequestEvent[P,L any] struct{}
func Load(any)int{return 0}
func Query(any)int{return 0}
type PrerenderOptions struct{Inputs any}
func Prerender(any,...PrerenderOptions)int{return 0}
func Command(any)int{return 0}
func Form(any)int{return 0}
func ActionNoData(func(context.Context)error)int{return 0}
func DefaultAction[Out any](func(context.Context)(Out,error))int{return 0}
`, imports).Types
	imports["example.com/app/internal/app"] = declarationPackage(t, "example.com/app/internal/app", "locals.go", "package app;type Locals struct{}", imports).Types
	imports["example.com/app/generated/params"] = declarationPackage(t, "example.com/app/generated/params", "params.go", `package params
import("github.com/tylergannon/skgo";"example.com/app/internal/app")
type Params struct{}
type RequestEvent=skgo.RequestEvent[Params,app.Locals]
type Resolve=skgo.RequestResolve[Params,app.Locals]
type Middleware=skgo.RequestMiddleware[Params,app.Locals]
`, imports).Types
	pkg := declarationPackage(t, "example.com/app/route", filename, source, imports)
	var call *ast.CallExpr
	ast.Inspect(pkg.Syntax[0], func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if _, ok := c.Fun.(*ast.SelectorExpr); ok {
				call = c
			}
		}
		return true
	})
	if call == nil {
		t.Fatal("missing marker call")
	}
	root := t.TempDir()
	writeSharedFixture(t, root, "go.mod", "module example.com/app\n\ngo 1.27\n")
	dir := filepath.Join(root, "web/src/routes/route")
	a := &app{cfg: Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated"), LocalsPackage: "example.com/app/internal/app", LocalsType: "Locals", loadParams: map[string]*routeLoadParams{dir: {}}}}
	return a, &goPackage{dir: dir, pkg: pkg}, pkg, call
}

const eventDeclarations = `
type RouteParams struct{}
type LayoutParams struct{}
type OtherParams struct{}
type PageRequestEvent=skgo.RequestEvent[RouteParams,app.Locals]
type LayoutRequestEvent=skgo.RequestEvent[LayoutParams,app.Locals]
`

func TestEventDomainDeclarations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ file, marker, event, want string }{
		{"page.server.go", "Load", "LayoutRequestEvent", "generated PageRequestEvent"},
		{"layout.server.go", "Load", "PageRequestEvent", "generated LayoutRequestEvent"},
		{"page.server.go", "Load", "params.RequestEvent", "generated PageRequestEvent"},
		{"layout.server.go", "Load", "params.RequestEvent", "generated LayoutRequestEvent"},
		{"page.server.go", "Load", "skgo.RequestEvent[RouteParams,struct{}]", "generated PageRequestEvent"},
		{"layout.server.go", "Load", "skgo.RequestEvent[LayoutParams,struct{}]", "generated LayoutRequestEvent"},
		{"page.server.go", "Load", "skgo.RequestEvent[OtherParams,app.Locals]", "generated PageRequestEvent"},
		{"data.remote.go", "Command", "PageRequestEvent", "generated/params.Params"},
		{"data.remote.go", "Form", "PageRequestEvent", "generated/params.Params"},
		{"data.remote.go", "Command", "skgo.RequestEvent[params.Params,struct{}]", "event locals must be"},
		{"data.remote.go", "Form", "skgo.RequestEvent[params.Params,struct{}]", "event locals must be"},
		{"page.server.go", "Load", "PageRequestEvent", ""},
		{"layout.server.go", "Load", "LayoutRequestEvent", ""},
		{"data.remote.go", "Command", "params.RequestEvent", ""},
		{"data.remote.go", "Form", "params.RequestEvent", ""},
	} {
		t.Run(tc.marker+"/"+tc.file+"/"+tc.event, func(t *testing.T) {
			args := tc.event
			if tc.marker != "Load" {
				args = "context.Context," + args + ",string"
			}
			source := `package route
import("context";"github.com/tylergannon/skgo";"example.com/app/internal/app";"example.com/app/generated/params")
var _=context.Background;var _=params.Params{}
` + eventDeclarations + "func target(" + args + ")(string,error){return \"\",nil}\nvar _=skgo." + tc.marker + "(target)\n"
			a, gp, p, call := markerDeclaration(t, tc.file, source)
			_, _, _, _, err := a.readMarker(gp, p, call, "src/routes/route/+page.server.ts", "", "/route", filepath.Join(gp.dir, tc.file))
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.file+":") {
				t.Fatalf("wanted positioned %q: %v", tc.want, err)
			}
		})
	}
}

func wireDeclaration(t *testing.T, source string) *packages.Package {
	t.Helper()
	imports := declarationImports{}
	imports[skgoFilePkg] = declarationPackage(t, skgoFilePkg, "file.go", "package formdata;type File struct{}", imports).Types
	imports[skgoPkg] = declarationPackage(t, skgoPkg, "skgo.go", `package skgo
import "github.com/tylergannon/skgo/internal/formdata"
type File = formdata.File
type Deferred[T any] struct{}
`, imports).Types
	return declarationPackage(t, "fixture", "wire.go", source, imports)
}

func TestMarkerRefusalsNameTheAuthoredDeclaration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, want string }{
		{`package data
import "github.com/tylergannon/skgo"
func broken(string)(string,error){return "",nil}
var _=skgo.Query(broken)
`, "broken is declared as a query"},
		{`package data
import("context";"github.com/tylergannon/skgo")
func build(context.Context,string)(string,error){return "",nil}
func producer()([]int,error){return nil,nil}
var _=skgo.Prerender(build,skgo.PrerenderOptions{Inputs:producer})
`, "invalid PrerenderOptions.Inputs producer"},
	} {
		a, gp, p, call := markerDeclaration(t, "data.remote.go", tc.source)
		_, _, _, _, err := a.readMarker(gp, p, call, "src/data.remote.ts", "", "", filepath.Join(gp.dir, "data.remote.go"))
		if err == nil || !strings.Contains(err.Error(), "data.remote.go:") || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("lost real positioned marker diagnostic: %v", err)
		}
	}
}
