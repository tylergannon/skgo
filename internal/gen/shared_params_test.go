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

func sharedParamsFixture(cfg Config, routes ...string) map[string]*routeLoadParams {
	all := map[string]*routeLoadParams{
		"src/routes/number/[id=Number]": {params: []skgo.ManifestParam{{Name: "id", Matcher: "Number"}}, matchers: map[string]goParamMatcher{"Number": {out: types.Typ[types.Int]}}},
		"src/routes/text/[id]":          {params: []skgo.ManifestParam{{Name: "id"}}},
		"src/routes/optional/[[slug]]":  {params: []skgo.ManifestParam{{Name: "slug", Optional: true}}},
		"src/routes/required/[slug]":    {params: []skgo.ManifestParam{{Name: "slug"}}},
		"src/routes/about":              {},
		"src/routes/contact":            {},
		"src/routes/page/[page]":        {params: []skgo.ManifestParam{{Name: "page"}}},
	}
	out := map[string]*routeLoadParams{}
	for _, route := range routes {
		out[filepath.Join(cfg.Web, filepath.FromSlash(route))] = all[route]
	}
	return out
}

func readSharedParams(t *testing.T, cfg Config, routes map[string]*routeLoadParams) string {
	t.Helper()
	if err := writeSharedParams(cfg, routes); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(cfg.Out, "params", sharedParamsFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestSharedParameterEmitter(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]")
	source := readSharedParams(t, cfg, sharedParamsFixture(cfg, "src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]"))
	got := strings.Join(strings.Fields(source), " ")
	for _, want := range []string{
		`type IDParam_Int struct{ Value int }`,
		`type IDParam_String struct{ Value string }`,
		`type Key_ID interface{ skgoParam_6964() }`,
		`func (p Params) ID() Key_ID { return p.value_6964 }`,
		`if raw, present := values[param.Name]; present`,
		`routeID, declared, values := skgo.RemoteCallerValues(event)`,
		`id, declared, values := skgo.HookValues(event)`,
		`has no generated variant; regenerate params`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("shared params lack %q:\n%s", want, source)
		}
	}
	for _, route := range []string{"/number", "/text", "/optional"} {
		if strings.Contains(source, route) {
			t.Fatalf("shared params name route %s; route membership must not change Go:\n%s", route, source)
		}
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "params.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	unquote := func(clause *ast.CaseClause) string {
		literal, ok := clause.List[0].(*ast.BasicLit)
		if !ok {
			t.Fatalf("case %T is not a literal", clause.List[0])
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	variants := map[string]string{}
	for _, declaration := range parsed.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "SkgoParams" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			names, ok := node.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, nameStatement := range names.Body.List {
				nameClause := nameStatement.(*ast.CaseClause)
				name := unquote(nameClause)
				for _, inner := range nameClause.Body {
					matchers, ok := inner.(*ast.SwitchStmt)
					if !ok {
						continue
					}
					for _, matcherStatement := range matchers.Body.List {
						matcherClause := matcherStatement.(*ast.CaseClause)
						key := name + "=" + unquote(matcherClause)
						ast.Inspect(matcherClause, func(child ast.Node) bool {
							if value, ok := child.(*ast.CompositeLit); ok {
								if ident, ok := value.Type.(*ast.Ident); ok {
									variants[key] = ident.Name
								}
							}
							return true
						})
					}
				}
			}
			return false
		})
	}
	want := map[string]string{"id=Number": "IDParam_Int", "id=": "IDParam_String", "slug=": "SlugParam_String"}
	if !reflect.DeepEqual(variants, want) {
		t.Fatalf("declaration constructors=%v want %v", variants, want)
	}
}

// Adding, removing or re-flagging a route that declares no new name and
// matcher must leave the shared params byte-identical: the development host
// then publishes the route with a reload instead of rebuilding Go.
func TestSharedParamsIgnoreRouteMembership(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/number/[id=Number]")
	base := readSharedParams(t, cfg, sharedParamsFixture(cfg, "src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]"))
	for name, routes := range map[string][]string{
		"svelte-only routes added":   {"src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]", "src/routes/about", "src/routes/contact"},
		"known declaration reused":   {"src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]", "src/routes/required/[slug]"},
		"duplicate declaration gone": {"src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/required/[slug]"},
	} {
		if got := readSharedParams(t, cfg, sharedParamsFixture(cfg, routes...)); got != base {
			t.Fatalf("%s changed shared params:\n--- before\n%s\n--- after\n%s", name, base, got)
		}
	}
	grown := readSharedParams(t, cfg, sharedParamsFixture(cfg, "src/routes/number/[id=Number]", "src/routes/text/[id]", "src/routes/optional/[[slug]]", "src/routes/page/[page]"))
	if grown == base || !strings.Contains(grown, "PageParam_String") {
		t.Fatalf("a new declaration must add its variant:\n%s", grown)
	}
}

func TestSharedParamsWithoutDeclarations(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t)
	source := readSharedParams(t, cfg, sharedParamsFixture(cfg, "src/routes/about"))
	if !strings.Contains(source, "if len(declared) != 0 {") || strings.Contains(source, "/about") {
		t.Fatalf("parameterless shared params:\n%s", source)
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
