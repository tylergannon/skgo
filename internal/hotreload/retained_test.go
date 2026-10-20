package hotreload

import (
	"context"
	"errors"
	"golang.org/x/tools/go/packages"
	"os"
	"path/filepath"
	"testing"
)

// Reverse callers must be source checked even when dependency type information
// comes from retained exports. Their direct calls prevent native replacement.
func TestRetainedApplicationCallerAndDependencyGuards(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{}
	write := func(name, source string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		files[path] = []byte(source)
	}
	write("go.mod", "module example.com/retained\n\ngo 1.27.1\n")
	write("dependency/value.go", "package dependency\nconst Offset=1\n")
	before := []byte("package target\nimport \"example.com/retained/dependency\"\nfunc Value()int{return 4101+dependency.Offset}\n")
	write("target/value.go", string(before))
	write("caller/caller.go", "package caller\nimport \"example.com/retained/target\"\nfunc Call()int{return target.Value()}\n")
	q := &retainedApplication{files: files, analysis: &retainedAnalysis{}}
	defer q.analysis.close()
	cfg := &packages.Config{Context: context.Background(), Dir: root, Env: append(os.Environ(), "GOWORK=off"), Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}
	if _, err := q.Load(cfg, "example.com/retained/target", "./..."); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "target/value.go")
	after := []byte("package target\nimport \"example.com/retained/dependency\"\nfunc Value()int{return 5202+dependency.Offset}\n")
	files[path] = after
	loaded, err := q.Load(cfg, "example.com/retained/target", "./...")
	if err != nil {
		t.Fatal(err)
	}
	var target *packages.Package
	var all []*packages.Package
	packages.Visit(loaded, nil, func(p *packages.Package) {
		all = append(all, p)
		if p.PkgPath == "example.com/retained/target" {
			target = p
		}
	})
	if target == nil {
		t.Fatal("target missing")
	}
	err = Classify(map[string][]byte{path: before}, map[string][]byte{path: after}, []*packages.Package{target}, all)
	var rebuild *CompleteBuild
	if !errors.As(err, &rebuild) {
		t.Fatalf("reverse caller admitted: %v", err)
	}
	files[filepath.Join(root, "dependency/value.go")] = []byte("package dependency\nconst Offset=9\n")
	if _, err = q.Load(cfg, "example.com/retained/target", "./..."); !errors.As(err, &rebuild) {
		t.Fatalf("changed dependency admitted against host archives: %v", err)
	}
}
