package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/tylergannon/skgo"
	"golang.org/x/tools/go/packages"
)

func writeSharedFixture(t *testing.T, app, path, source string) {
	t.Helper()
	path = filepath.Join(app, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSharedParameterEmitter(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]")
	routes := map[string]*routeLoadParams{
		filepath.Join(cfg.Web, "src/routes/number/[id=Number]"): {params: []skgo.ManifestParam{{Name: "id", Matcher: "Number"}}, matchers: map[string]goParamMatcher{"Number": {out: types.Typ[types.Int]}}},
		filepath.Join(cfg.Web, "src/routes/text/[id]"):          {params: []skgo.ManifestParam{{Name: "id"}}},
		filepath.Join(cfg.Web, "src/routes/optional/[[slug]]"):  {params: []skgo.ManifestParam{{Name: "slug", Optional: true}}},
	}
	if err := writeSharedParams(cfg, routes); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(cfg.Out, "params", sharedParamsFile))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strings.Fields(string(source)), " ")
	for _, want := range []string{
		`type IDParam_Int struct{ Value int }`,
		`type IDParam_String struct{ Value string }`,
		`type Key_ID interface{ skgoParam_6964() }`,
		`func (p Params) ID() Key_ID { return p.value_6964 }`,
		`case "/number/[id=Number]":`,
		`p.value_6964 = IDParam_Int{Value: value}`,
		`case "/text/[id]":`,
		`p.value_6964 = IDParam_String{Value: value}`,
		`if raw, present := values["slug"]; present`,
		`routeID, values := skgo.RemoteCallerValues(event)`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("shared params lack %q:\n%s", want, source)
		}
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "params.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	branches := map[string]string{}
	for _, declaration := range parsed.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "SkgoParams" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			branch, ok := node.(*ast.CaseClause)
			if !ok || len(branch.List) != 1 {
				return true
			}
			literal, ok := branch.List[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			route, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(branch, func(child ast.Node) bool {
				value, ok := child.(*ast.CompositeLit)
				if !ok {
					return true
				}
				name, ok := value.Type.(*ast.Ident)
				if ok && strings.HasPrefix(name.Name, "IDParam_") {
					branches[route] = name.Name
				}
				return true
			})
			return false
		})
	}
	wantBranches := map[string]string{"/number/[id=Number]": "IDParam_Int", "/text/[id]": "IDParam_String"}
	if !reflect.DeepEqual(branches, wantBranches) {
		t.Fatalf("route constructors=%v want %v", branches, wantBranches)
	}
}

func TestSharedParameterDomainPackageBoundaries(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/owned")
	writeSharedFixture(t, cfg.Web, "src/data/value.remote.go", "package data\n")
	for _, tc := range []struct{ path, dependency, want string }{
		{"example.com/app/domain", "", ""},
		{"example.com/app/foreign/internal/domain", "", "cannot import internal type"},
		{"example.com/app/web/src/data", "", "caller-owned"},
		{"example.com/app/web/src/routes/owned", "", "caller-owned"},
		{"example.com/app/generated/params", "", "caller-owned"},
		{"example.com/app/domain", "example.com/app/generated/params", "imports generated bindings/params"},
	} {
		t.Run(tc.path+tc.dependency, func(t *testing.T) {
			pkg := types.NewPackage(tc.path, "domain")
			typ := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Value", nil), types.Typ[types.Int], nil)
			metadata := &packages.Package{PkgPath: tc.path, Types: pkg, Imports: map[string]*packages.Package{}}
			if tc.dependency != "" {
				metadata.Imports[tc.dependency] = &packages.Package{PkgPath: tc.dependency}
			}
			err := validateSharedType(cfg, typ, map[string]*packages.Package{tc.path: metadata})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
		})
	}
}
