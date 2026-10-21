package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestFunctionDescription(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "remote.go", "package sample\n// read returns the current task list.\nfunc read() {}\nfunc undocumented() {}", parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	read := file.Decls[0].(*ast.FuncDecl)
	if got := functionDescription([]*ast.File{file}, read.Name.Pos()); got != "read returns the current task list." {
		t.Fatalf("description=%q", got)
	}
	if got := functionDescription([]*ast.File{file}, file.Decls[1].(*ast.FuncDecl).Name.Pos()); got != "" {
		t.Fatalf("undocumented=%q", got)
	}
}
