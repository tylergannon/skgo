package probe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/gopls/internal/cache/metadata"
	"golang.org/x/tools/gopls/internal/cache/parsego"
	"golang.org/x/tools/gopls/internal/cache/typerefs"
)

const source = `package model
import "example.test/storage"
type Record struct { ID storage.Key ` + "`json:\"id\"`" + ` }
type Alias = storage.Record
type Block [storage.Size]byte
var Inferred = storage.NewRecord()
func Handle(r Record) Record { storage.Audit(); return r }
func Workflow() { storage.Audit() }
`

// Run the pinned upstream algorithm, not a reimplementation of it.
func refs(t testing.TB, src string) map[string][]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "model.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[metadata.ImportPath]*metadata.Package{
		"example.test/storage": {ID: "example.test/storage", Name: "storage"},
	}
	encoded := typerefs.Encode([]*parsego.File{{File: f}}, imports)
	index := typerefs.NewPackageIndex()
	out := map[string][]string{}
	for _, class := range typerefs.Decode(index, encoded) {
		var names []string
		for _, ref := range class.Refs {
			names = append(names, string(index.DeclaringPackage(ref))+"."+ref.Name)
		}
		slices.Sort(names)
		for _, decl := range class.Decls {
			out[decl] = names
		}
	}
	return out
}

func TestUpstreamTypeReachability(t *testing.T) {
	want := map[string][]string{
		"Record":   {"example.test/storage.Key"},
		"Alias":    {"example.test/storage.Record"},
		"Block":    {"example.test/storage.Size"},
		"Inferred": {"example.test/storage.NewRecord"},
		"Handle":   {"example.test/storage.Key"},
	}
	if got := refs(t, source); !reflect.DeepEqual(got, want) {
		t.Fatalf("upstream refs = %#v, want %#v", got, want)
	}
}

func TestBodyEdgesAreAbsent(t *testing.T) {
	// A workflow's called function changes; its signature does not. Type
	// reachability deliberately ignores this edit. A workflow generator cannot.
	changed := strings.Replace(source, "func Workflow() { storage.Audit() }", "func Workflow() { storage.Publish() }", 1)
	if !reflect.DeepEqual(refs(t, source), refs(t, changed)) {
		t.Fatal("body-only edit unexpectedly changes type reachability")
	}
	if _, ok := refs(t, changed)["Workflow"]; ok {
		t.Fatal("body-only dependency unexpectedly appears in upstream type graph")
	}
}

func TestProjectionChangesAreNotGraphChanges(t *testing.T) {
	// JSON tags affect emitted TypeScript/JSON but not graph topology. Reusing
	// an output solely because typerefs is unchanged would return stale output.
	changed := strings.Replace(source, `json:"id"`, `json:"record_id"`, 1)
	if changed == source {
		t.Fatal("fixture did not change")
	}
	if !reflect.DeepEqual(refs(t, source), refs(t, changed)) {
		t.Fatal("field-tag edit unexpectedly changes type reachability")
	}
}

func TestSignatureChangesReachability(t *testing.T) {
	changed := strings.Replace(source, "func Handle(r Record) Record", "func Handle(r storage.Result) storage.Result", 1)
	if got := refs(t, changed)["Handle"]; !reflect.DeepEqual(got, []string{"example.test/storage.Result"}) {
		t.Fatalf("changed signature refs = %v", got)
	}
}

type importPackage func(string) (*types.Package, error)

func (f importPackage) Import(path string) (*types.Package, error) { return f(path) }

func TestTypedConstantValueNeedsAdditionalTracking(t *testing.T) {
	const model = `package model; import "example.test/storage"; const Explicit int = storage.Code; type Block [Explicit]byte`
	// Upstream's type graph visits an explicitly typed value's type, omitting
	// its initializer. A generator exposing enum values needs that edge too.
	if got := refs(t, model); len(got) != 0 {
		t.Fatalf("unexpected type references: %v", got)
	}
	for _, literal := range []string{"1", "2"} {
		fset := token.NewFileSet()
		dependency, err := parser.ParseFile(fset, "storage.go", "package storage; const Code = "+literal, 0)
		if err != nil {
			t.Fatal(err)
		}
		storage, err := new(types.Config).Check("example.test/storage", fset, []*ast.File{dependency}, nil)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, "model.go", model, 0)
		if err != nil {
			t.Fatal(err)
		}
		cfg := types.Config{Importer: importPackage(func(string) (*types.Package, error) { return storage, nil })}
		pkg, err := cfg.Check("example.test/model", fset, []*ast.File{file}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := pkg.Scope().Lookup("Explicit").(*types.Const).Val().ExactString(); got != literal {
			t.Fatalf("constant = %s, want %s", got, literal)
		}
		wantLength := int64(1)
		if literal == "2" {
			wantLength = 2
		}
		if got := pkg.Scope().Lookup("Block").Type().Underlying().(*types.Array).Len(); got != wantLength {
			t.Fatalf("array length = %d, want %d", got, wantLength)
		}
	}
}

func BenchmarkUpstreamParseAndTypeReferences(b *testing.B) {
	for b.Loop() {
		refs(b, source)
	}
}
