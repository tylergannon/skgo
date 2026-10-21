package skgo_test

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// go/doc concatenates the package comment of every file in the package, so a
// file-level essay attached to its `package` clause lands in the overview that
// `go doc` and pkg.go.dev show. doc.go owns the overview; any other file's
// explanation is an ordinary comment set off by a blank line.
func TestPackageOverviewComesFromDocGoOnly(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	var withPackageComment []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = file
		if file.Doc != nil {
			withPackageComment = append(withPackageComment, name)
		}
	}
	slices.Sort(withPackageComment)
	if want := []string{"doc.go"}; !slices.Equal(withPackageComment, want) {
		t.Fatalf("files with a package comment = %v, want %v", withPackageComment, want)
	}

	// doc.New consumes the files' comments, so read doc.go's first.
	want := files["doc.go"].Doc.Text()
	pkg := doc.New(&ast.Package{Name: "skgo", Files: files}, "github.com/tylergannon/skgo", doc.AllDecls)
	if pkg.Doc != want {
		t.Fatalf("go/doc overview is not doc.go's comment alone:\n%s", pkg.Doc)
	}
	if !strings.HasPrefix(pkg.Doc, "Package skgo is a Go backend for SvelteKit.") {
		t.Fatalf("go/doc overview starts %q", pkg.Doc[:min(len(pkg.Doc), 60)])
	}
}
