package gen

import (
	"go/token"
	"go/types"
	"path/filepath"
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
			got, err := (&app{}).project(tc.in)
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
			got, err := (&app{}).project(tc.in)
			if err == nil {
				t.Fatalf("project(%s) = %q, want an error", tc.in, got.expr)
			}
			if !strings.Contains(err.Error(), tc.about) {
				t.Fatalf("project(%s) failed with %q, want it to explain the %s", tc.in, err, tc.about)
			}
		})
	}
}

// TestImportPathOutsideTheRouteTreeStillHasToBeNameable: the link tree covers
// route directories, whose names SvelteKit chooses. Anywhere else the developer
// chose the name, and Go's rules apply.
func TestImportPathOutsideTheRouteTreeStillHasToBeNameable(t *testing.T) {
	a := &app{
		cfg:        Config{Web: filepath.Join("/app", "web")},
		hostDir:    "/app",
		hostModule: "example.com/app",
	}

	if got, err := a.importPath("/app/web/src/lib"); err != nil || got != "example.com/app/web/src/lib" {
		t.Fatalf("importPath(lib) = %q, %v; want the plain import path", got, err)
	}

	_, err := a.importPath("/app/web/src/lib/(shared)")
	if err == nil {
		t.Fatal("importPath accepted a library directory Go cannot name")
	}
	if !strings.Contains(err.Error(), "Go can spell") {
		t.Fatalf("error = %q, want it to say what is wrong with the name", err)
	}
}
