package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestSerializedFieldNames(t *testing.T) {
	t.Parallel()
	const source = `package fixture
type bad struct {
	A string ` + "`json:\"same\"`" + `
	B string ` + "`json:\"same\"`" + `
}
type good struct {
	A string ` + "`json:\"first\"`" + `
	B string ` + "`json:\"second\"`" + `
	Skip string ` + "`json:\"-\"`" + `
}
type nested struct { Value good ` + "`json:\"value\"`" + ` }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "wire.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("fixture", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bad := pkg.Scope().Lookup("bad").Type()
	if err := checkTypeSerializedNames(bad, fset, nil); err == nil || !strings.Contains(err.Error(), "wire.go:4:") || !strings.Contains(err.Error(), `duplicate serialized name "same"`) {
		t.Fatalf("bad wire shape: %v", err)
	}
	for _, name := range []string{"good", "nested"} {
		if err := checkTypeSerializedNames(pkg.Scope().Lookup(name).Type(), fset, nil); err != nil {
			t.Fatalf("valid %s: %v", name, err)
		}
	}
}
