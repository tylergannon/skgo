package gen

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/polytype/grammar"
	"golang.org/x/tools/go/packages"
)

func TestPackageSnapshotCompatibility(t *testing.T) {
	root := t.TempDir()
	write := func(name, source string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module snapshot.test\n\ngo 1.27.1\n")
	write("types.go", "package example\n// Value carries a comment.\ntype Value struct { Text string }\n")
	write("tagged.go", "//go:build other\n\npackage example\ntype Other string\n")
	config := &packages.Config{Dir: root, Mode: grammar.LoadMode | packages.NeedDeps}
	roots, err := packages.Load(config, "snapshot.test")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewPackageSnapshot(config, roots...)
	if err != nil {
		t.Fatal(err)
	}
	fail := Config{Packages: []*PackageSnapshot{snapshot}, LoadPackages: func(*packages.Config, ...string) ([]*packages.Package, error) {
		return nil, fmt.Errorf("unexpected load")
	}}
	got, err := fail.loadPackages(config, "snapshot.test")
	if err != nil || got[0] != roots[0] {
		t.Fatalf("not reused: %v", err)
	}
	changed := *config
	changed.BuildFlags = []string{"-tags=jsonschema"}
	if _, err := fail.loadPackages(&changed, "snapshot.test"); err == nil {
		t.Fatal("reused incompatible tags")
	}
	changed = *config
	changed.Overlay = map[string][]byte{filepath.Join(root, "types.go"): []byte("package example\ntype Value int\n")}
	if _, err := fail.loadPackages(&changed, "snapshot.test"); err == nil {
		t.Fatal("reused incompatible overlay")
	}
	write("tagged.go", "package example\ntype Other string\n")
	if _, err := fail.loadPackages(config, "snapshot.test"); err == nil {
		t.Fatal("reused changed excluded file")
	}
	write("tagged.go", "//go:build other\n\npackage example\ntype Other string\n")
	write("types.go", "package example\ntype Value struct { Changed bool }\n")
	if _, err := fail.loadPackages(config, "snapshot.test"); err == nil {
		t.Fatal("reused edited source")
	}
}

func TestPreparedGenerationAndCallerPackages(t *testing.T) {
	root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
// Reply documents the response.
type Reply struct { Text string }
func query(context.Context)(Reply,error){return Reply{},nil}
var _=skgo.Query(query)
`, nil)
	var observed int
	cfg.OnPackageLoad = func(*packages.Config, []string) { observed++ }
	p, err := Prepare(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if observed == 0 {
		t.Fatal("no analysis loads observed")
	}
	graph := p.Packages()
	if len(graph) != 1 || graph[0].Types.Scope().Lookup("Reply") == nil {
		t.Fatal("prepared graph missing authored type")
	}
	before := observed
	if err := p.Run(); err != nil {
		t.Fatal(err)
	}
	if observed <= before {
		t.Fatal("expected separate jsonschema projection load")
	}
	p.Close()
	p.Close()
	if err := p.Run(); err == nil {
		t.Fatal("reused closed preparation")
	}
	output := filepath.Join(root, "app/web/src/data/data.remote.ts")
	ordinary, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	host := filepath.Join(root, "app")
	cfg.OnPackageLoad = nil
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	same, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ordinary, same) {
		t.Fatal("prepared and ordinary outputs differ")
	}

	source := filepath.Join(host, "web/src/data/data.remote.go")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, bytes.ReplaceAll(raw, []byte("Text string"), []byte("Changed bool")), 0644); err != nil {
		t.Fatal(err)
	}
	next, err := Prepare(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if !strings.Contains(next.Packages()[0].Types.Scope().Lookup("Reply").Type().Underlying().String(), "Changed bool") {
		t.Fatal("next invocation missed source edit")
	}
	if err := next.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestSuppliedPhasePackagesAvoidAllLoads(t *testing.T) {
	root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
// Reply keeps its comment through supplied packages.
type Reply struct { Text string }
func query(context.Context)(Reply,error){return Reply{},nil}
var _=skgo.Query(query)
`, map[string]string{"app/hooks/hooks.go": `package hooks
import("context";"net/http";"example.com/app/generated/params")
func Handle(ctx context.Context,event params.RequestEvent,resolve params.Resolve)(*http.Response,error){return resolve(ctx,event)}
`})
	cfg.HookPackage = "example.com/app/hooks"
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	expected := generatedTree(t, root)
	var snapshots []*PackageSnapshot
	var phases []string
	cfg.LoadPackages = func(config *packages.Config, patterns ...string) ([]*packages.Package, error) {
		config.Mode |= grammar.LoadMode | packages.NeedDeps
		roots, err := packages.Load(config, patterns...)
		if err != nil {
			return nil, err
		}
		snapshot, err := NewPackageSnapshot(config, roots...)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
		phases = append(phases, strings.Join(patterns, ","))
		return roots, nil
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if len(snapshots) < 4 {
		t.Fatalf("missing phase coverage: %v", phases)
	}
	cfg.Packages = snapshots
	cfg.LoadPackages = func(config *packages.Config, patterns ...string) ([]*packages.Package, error) {
		return nil, fmt.Errorf("unexpected reload %v (flags %v)", patterns, config.BuildFlags)
	}
	prepared, err := Prepare(cfg)
	if err != nil {
		t.Fatal(err)
	}
	foundComment := false
	for _, pkg := range prepared.Packages() {
		for _, file := range pkg.Syntax {
			for _, comment := range file.Comments {
				if strings.Contains(comment.Text(), "Reply keeps its comment") {
					foundComment = true
				}
			}
		}
	}
	if !foundComment {
		t.Fatal("supplied AST lost declaration comments")
	}
	if err := prepared.Run(); err != nil {
		t.Fatal(err)
	}
	prepared.Close()
	if !reflect.DeepEqual(expected, generatedTree(t, root)) {
		t.Fatal("supplied package output differs from ordinary loading")
	}
	// Old snapshots must not prevent recovery after deleting generated output.
	path := filepath.Join(root, "app/web/src/data/skgo_gen.go")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	cfg.LoadPackages = nil
	fallback := 0
	cfg.OnPackageLoad = func(*packages.Config, []string) { fallback++ }
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if fallback == 0 {
		t.Fatal("missing generated source reused a stale snapshot")
	}
	if !reflect.DeepEqual(expected, generatedTree(t, root)) {
		t.Fatal("missing generated output was not recovered")
	}
	// Reusing the old bundle after an authored edit must see the new contract.
	source := filepath.Join(root, "app/web/src/data/data.remote.go")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, bytes.ReplaceAll(raw, []byte("Text string"), []byte("Changed bool")), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(filepath.Join(root, "app/web/src/data/types.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("Changed")) || bytes.Contains(output, []byte("Text:")) {
		t.Fatalf("stale projection after edit: %s", output)
	}
}

func generatedTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if generatedArtifact(data) {
			result[path] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSuppliedPackagesPreserveDiagnostics(t *testing.T) {
	root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func query(context.Context)(string,error){return "",nil}
var _=skgo.Query(query)
`, nil)
	if err := os.WriteFile(filepath.Join(root, "app/internal/app/locals.go"), []byte("package app\ntype Locals string\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ordinary := Run(cfg)
	if ordinary == nil {
		t.Fatal("invalid locals accepted")
	}
	config := &packages.Config{Dir: filepath.Join(root, "app"), Mode: grammar.LoadMode | packages.NeedDeps}
	roots, err := packages.Load(config, cfg.LocalsPackage)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewPackageSnapshot(config, roots...)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Packages = []*PackageSnapshot{snapshot}
	cfg.LoadPackages = func(*packages.Config, ...string) ([]*packages.Package, error) {
		return nil, fmt.Errorf("unexpected reload")
	}
	supplied := Run(cfg)
	if supplied == nil || supplied.Error() != ordinary.Error() {
		t.Fatalf("diagnostics differ: ordinary %v; supplied %v", ordinary, supplied)
	}
}
