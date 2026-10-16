package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// Dev can retain a consuming package's export-data types while checking the
// hooks package from source. Those objects describe the same Go declaration,
// but do not share pointers. The browser still receives the decoded type.
func TestTransportTypeIdentityAcrossAnalysis(t *testing.T) {
	check := func(path string) *types.Package {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "hooks.go", `package hooks
 type RunSnapshot struct { JSON string `+"`json:\"json\"`"+` }
 type Box[T any] struct { Value T }
 type Data struct { Snapshot RunSnapshot `+"`json:\"snapshot\"`"+` }
 `, 0)
		if err != nil {
			t.Fatal(err)
		}
		pkg, err := new(types.Config).Check(path, fset, []*ast.File{file}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return pkg
	}
	marker := check("example.com/app/hooks")
	consumer := check("example.com/app/hooks")
	other := check("example.com/other/hooks")
	lookup := func(pkg *types.Package, name string) *types.Named {
		return pkg.Scope().Lookup(name).Type().(*types.Named)
	}
	snapshot := lookup(marker, "RunSnapshot")
	if snapshot.Obj() == lookup(consumer, "RunSnapshot").Obj() {
		t.Fatal("fixture did not create independent analyses")
	}
	a := &app{transported: []*transportedType{{key: "Snapshot", named: snapshot}}}
	for _, tc := range []struct {
		name string
		typ  types.Type
		want string
	}{
		{"same declaration", lookup(consumer, "RunSnapshot"), "RunSnapshot"},
		{"containing struct", lookup(consumer, "Data"), "{ snapshot: RunSnapshot }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.project(tc.typ)
			if err != nil || got.expr != tc.want || len(got.transported) != 1 || len(got.deps) != 0 {
				t.Fatalf("decoded result = %+v, %v; want %s imported from hooks", got, err, tc.want)
			}
		})
	}
	if _, ok := a.transportedNamed(lookup(other, "RunSnapshot")); ok {
		t.Fatal("same name from a different package became transported")
	}
	box := func(pkg *types.Package, arg types.Type) *types.Named {
		t.Helper()
		typ, err := types.Instantiate(nil, lookup(pkg, "Box"), []types.Type{arg}, true)
		if err != nil {
			t.Fatal(err)
		}
		return typ.(*types.Named)
	}
	a.transported = []*transportedType{{key: "IntBox", named: box(marker, types.Typ[types.Int])}}
	if _, ok := a.transportedNamed(box(consumer, types.Typ[types.Int])); !ok {
		t.Fatal("same instantiated declaration lost transport")
	}
	if _, ok := a.transportedNamed(box(marker, types.Typ[types.String])); ok {
		t.Fatal("different generic argument became transported")
	}
	if _, ok := a.transportedNamed(box(other, types.Typ[types.Int])); ok {
		t.Fatal("different generic declaration became transported")
	}
}
