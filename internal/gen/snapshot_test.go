package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Snapshot admission is SKGO's decision. The caller already owns a checked
// graph; rebuilding a whole application does not strengthen this contract.
func TestPackageSnapshotAdmission(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	const source = "package fixture\ntype Reply struct { Text string }\n"
	path := filepath.Join(root, "types.go")
	cacheWrite(t, path, source)
	cacheWrite(t, filepath.Join(root, "go.mod"), "module snapshot.test\n\ngo 1.27\n")
	pkg := declarationPackage(t, "snapshot.test", path, source, nil)
	pkg.PkgPath = "snapshot.test"
	pkg.GoFiles = []string{path}
	pkg.Module = &packages.Module{Path: "snapshot.test", Main: true, Dir: root, GoMod: filepath.Join(root, "go.mod")}
	load := &packages.Config{Dir: root, Mode: packages.NeedModule | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax, Env: []string{"GOWORK=off", "GOPACKAGESDRIVER=off"}}
	snapshot, err := NewPackageSnapshot(load, pkg)
	if err != nil {
		t.Fatal(err)
	}
	fallbacks := 0
	cfg := Config{Packages: []*PackageSnapshot{snapshot}, LoadPackages: func(*packages.Config, ...string) ([]*packages.Package, error) {
		fallbacks++
		return nil, fmt.Errorf("fixture fallback")
	}}
	got, err := cfg.loadPackages(load, "snapshot.test")
	if err != nil || len(got) != 1 || got[0] != pkg || got[0].Types.Scope().Lookup("Reply") == nil || fallbacks != 0 {
		t.Fatalf("compatible graph not reused: %v %v, fallbacks %d", got, err, fallbacks)
	}
	changed := *load
	changed.BuildFlags = []string{"-tags=other"}
	if _, err = cfg.loadPackages(&changed, "snapshot.test"); err == nil || fallbacks != 1 {
		t.Fatalf("incompatible configuration bypassed fallback: %v, %d", err, fallbacks)
	}
	if err = os.WriteFile(path, []byte("package fixture\ntype Reply struct { Count int }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = cfg.loadPackages(load, "snapshot.test"); err == nil || fallbacks != 2 {
		t.Fatalf("edited source reused stale graph: %v, %d", err, fallbacks)
	}
}
