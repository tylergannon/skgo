package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestSharedParamsGoIdentities(t *testing.T) {
	const source = `package domain
 type Number int
 type Alias = Number
 type Other int
 type Box[T any] struct{ Value T }
 type IntBox = Box[int]
 type StringBox = Box[string]
 type F = func(number int) string
 type G = func(value int) (label string)
 type Variadic = func(...int) string
 type I = interface{ Label() string; Count() int }
 type J = interface{ Count() int; Label() string }
 type K = interface{ I }
 type Struct = struct{ Value int ` + "`json:\"value\"`" + ` }
 type Tagged = struct{ Value int ` + "`json:\"other\"`" + ` }
 type Embedded = struct{ Number }
 type Field = struct{ Number Number }
 type Pointer = *Number
 type OtherPointer = *Other
 type Channel = chan Number
 type Receive = <-chan Number
 type Byte = byte
 type Uint8 = uint8
 type Rune = rune
 type Int32 = int32
 `
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "domain.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("example.com/domain", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		a, b      string
		identical bool
	}{
		{"Number", "Alias", true}, {"Number", "Other", false}, {"F", "G", true}, {"F", "Variadic", false},
		{"I", "J", true}, {"I", "K", true}, {"Struct", "Tagged", false}, {"Embedded", "Field", false},
		{"Pointer", "OtherPointer", false}, {"Channel", "Receive", false}, {"IntBox", "StringBox", false}, {"Byte", "Uint8", true}, {"Rune", "Int32", true},
	} {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			a, b := pkg.Scope().Lookup(tc.a).Type(), pkg.Scope().Lookup(tc.b).Type()
			if got := types.Identical(a, b); got != tc.identical {
				t.Fatalf("fixture Go identity %t, want %t", got, tc.identical)
			}
			if got := sharedTypeIdentity(a) == sharedTypeIdentity(b); got != tc.identical {
				t.Fatalf("canonical identity %t, want %t:\n%s\n%s", got, tc.identical, sharedTypeIdentity(a), sharedTypeIdentity(b))
			}
			alts := map[string][]sharedAlternative{"id": {{a, sharedVariantName("id", a)}}}
			if !tc.identical {
				alts["id"] = append(alts["id"], sharedAlternative{b, sharedVariantName("id", b)})
			}
			nameSharedAlternatives(alts)
			if got := sharedAlternativeName(alts, "id", a) == sharedAlternativeName(alts, "id", b); got != tc.identical {
				t.Fatalf("variant identity %t, want %t", got, tc.identical)
			}
		})
	}
}

func TestSharedParamsKeyNames(t *testing.T) {
	for _, tc := range []struct{ key, want string }{
		{"id", "ID"}, {"slug", "Slug"}, {"params", "Params"}, {"request_event", "RequestEvent_K726571756573745f6576656e74"},
		{"slug_string", "SlugString_K736c75675f737472696e67"}, {"1", "Key1_K31"}, {"_", "Key_K5f"}, {"-", "Key_K2d"},
		{"a_b", "AB_K615f62"}, {"a-b", "AB_K612d62"}, {"iD", "ID_K6944"}, {"ID", "ID_K4944"}, {"é", "Key_Kc3a9"},
	} {
		if got := sharedParamStem(tc.key); got != tc.want || !token.IsIdentifier(got) {
			t.Fatalf("%q -> %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestSharedParamsReadableVariantNames(t *testing.T) {
	makeNamed := func(path, pkg, name string) types.Type {
		p := types.NewPackage(path, pkg)
		return types.NewNamed(types.NewTypeName(token.NoPos, p, name, nil), types.Typ[types.Int], nil)
	}
	sales := makeNamed("example.com/sales", "sales", "OrderNumber")
	legacy := makeNamed("example.com/legacy", "legacy", "OrderNumber")
	other := makeNamed("example.com/other-sales", "sales", "OrderNumber")
	build := func(ts ...types.Type) map[string][]sharedAlternative {
		a := map[string][]sharedAlternative{}
		for _, typ := range ts {
			a["number"] = append(a["number"], sharedAlternative{typ, sharedVariantName("number", typ)})
		}
		nameSharedAlternatives(a)
		return a
	}
	one := build(types.Typ[types.String], types.Typ[types.Int], sales)
	for typ, want := range map[types.Type]string{
		types.Typ[types.String]: "NumberParam_String", types.Typ[types.Int]: "NumberParam_Int", sales: "NumberParam_OrderNumber",
	} {
		if got := sharedAlternativeName(one, "number", typ); got != want {
			t.Fatalf("name %s, want %s", got, want)
		}
	}
	two := build(types.Typ[types.String], types.Typ[types.Int], sales, legacy)
	if got := sharedAlternativeName(two, "number", sales); got != "NumberParam_SalesOrderNumber" {
		t.Fatal(got)
	}
	if got := sharedAlternativeName(two, "number", legacy); got != "NumberParam_LegacyOrderNumber" {
		t.Fatal(got)
	}
	if got := sharedAlternativeName(two, "number", types.Typ[types.String]); got != "NumberParam_String" {
		t.Fatal(got)
	}
	three, reverse := build(sales, legacy, other), build(other, legacy, sales)
	for _, typ := range []types.Type{sales, legacy, other} {
		if sharedAlternativeName(three, "number", typ) != sharedAlternativeName(reverse, "number", typ) {
			t.Fatal("names depend on traversal order")
		}
	}
	if sharedAlternativeName(three, "number", sales) == sharedAlternativeName(three, "number", other) {
		t.Fatal("equal package names erased distinct type identity")
	}
}
