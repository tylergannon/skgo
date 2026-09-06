package gen

import (
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func named(pkgPath, pkgName, typeName string, underlying types.Type) *types.Named {
	pkg := types.NewPackage(pkgPath, pkgName)
	obj := types.NewTypeName(token.NoPos, pkg, typeName, nil)
	return types.NewNamed(obj, underlying, nil)
}

func todoStruct() *types.Struct {
	pkg := types.NewPackage("app/todos", "todos")
	return types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, pkg, "Text", types.Typ[types.String], false),
	}, []string{`json:"text"`})
}

// TestProjectSpellsTheShapesPolytypeSpells covers the types skgo renders
// itself: everything else is a named type, which polytype declares.
func TestProjectSpellsTheShapesPolytypeSpells(t *testing.T) {
	todo := named("app/todos", "todos", "Todo", todoStruct())
	status := named("app/todos", "todos", "Status", types.Typ[types.String])

	for _, tc := range []struct {
		name string
		in   types.Type
		want string
		deps []string
	}{
		{"string", types.Typ[types.String], "string", nil},
		{"bool", types.Typ[types.Bool], "boolean", nil},
		{"int", types.Typ[types.Int], "number", nil},
		{"float64", types.Typ[types.Float64], "number", nil},
		{"slice of strings", types.NewSlice(types.Typ[types.String]), "Array<string>", nil},
		{"named struct", todo, "Todo", []string{"Todo"}},
		{"slice of named structs", types.NewSlice(todo), "Array<Todo>", []string{"Todo"}},
		{"named string", status, "Status", []string{"Status"}},
		{"array", types.NewArray(types.Typ[types.Int], 3), "Array<number>", nil},
		{"time.Time", named("time", "time", "Time", types.NewStruct(nil, nil)), "string", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := project(tc.in)
			if err != nil {
				t.Fatalf("project(%s): %v", tc.in, err)
			}
			if got.expr != tc.want {
				t.Fatalf("project(%s) = %q, want %q", tc.in, got.expr, tc.want)
			}
			var deps []string
			for _, dep := range got.deps {
				deps = append(deps, dep.Obj().Name())
			}
			if strings.Join(deps, ",") != strings.Join(tc.deps, ",") {
				t.Fatalf("project(%s) depends on %v, want %v", tc.in, deps, tc.deps)
			}
		})
	}
}

// TestProjectRefusesWhatPolytypeCannotCarry pins the admission rule. Each of
// these either has no projection at all, or one polytype emits that
// encoding/json would then contradict — and a declaration that lies is worse
// than a build that stops.
func TestProjectRefusesWhatPolytypeCannotCarry(t *testing.T) {
	todo := named("app/todos", "todos", "Todo", todoStruct())

	for _, tc := range []struct {
		name  string
		in    types.Type
		about string
	}{
		{"map", types.NewMap(types.Typ[types.String], types.Typ[types.Int]), "map"},
		{"pointer", types.NewPointer(todo), "pointer"},
		{"slice of pointers", types.NewSlice(types.NewPointer(todo)), "pointer"},
		{"interface", types.NewInterfaceType(nil, nil), "interface"},
		{"named map", named("app/todos", "todos", "Index", types.NewMap(types.Typ[types.String], types.Typ[types.Int])), "map"},
		{"channel", types.NewChan(types.SendRecv, types.Typ[types.Int]), "projection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := project(tc.in)
			if err == nil {
				t.Fatalf("project(%s) = %q, want an error", tc.in, got.expr)
			}
			if !strings.Contains(err.Error(), tc.about) {
				t.Fatalf("project(%s) failed with %q, want it to explain the %s", tc.in, err, tc.about)
			}
		})
	}
}

// TestImportPathRefusesARouteDirectoryGoCannotName is the honest limit of
// colocation: `src/routes/todos/[id]` is not a legal Go import path, so no
// package can live there.
func TestImportPathRefusesARouteDirectoryGoCannotName(t *testing.T) {
	a := &app{hostDir: "/app", hostModule: "example.com/app"}

	if got, err := a.importPath("/app/web/src/routes/todos"); err != nil || got != "example.com/app/web/src/routes/todos" {
		t.Fatalf("importPath(todos) = %q, %v; want the plain import path", got, err)
	}

	_, err := a.importPath("/app/web/src/routes/todos/[id]")
	if err == nil {
		t.Fatal("importPath accepted a bracketed route directory")
	}
	if !strings.Contains(err.Error(), "brackets") {
		t.Fatalf("error = %q, want it to explain why the directory cannot hold Go", err)
	}
}
