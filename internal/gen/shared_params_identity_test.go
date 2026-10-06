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
			if got := sharedVariantName("id", a) == sharedVariantName("id", b); got != tc.identical {
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
