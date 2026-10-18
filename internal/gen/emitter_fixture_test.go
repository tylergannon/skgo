package gen

import (
	"go/types"
	"path/filepath"
	"testing"

	"github.com/tylergannon/polytype/grammar"
	"golang.org/x/tools/go/packages"
)

// Supply a small parsed type graph to emitters. No module loading, declaration
// discovery, source mirroring or subprocess is needed to inspect their output.
func emitterPackage(t *testing.T, root, source string) (*app, *goPackage) {
	t.Helper()
	dir := filepath.Join(root, "data")
	path := filepath.Join(dir, "data.go")
	writeSharedFixture(t, root, "go.mod", "module example.com/app\n\ngo 1.27\n")
	p := declarationPackage(t, "example.com/app/data", path, source, nil)
	p.PkgPath = p.Types.Path()
	p.GoFiles = []string{path}
	p.CompiledGoFiles = p.GoFiles
	loaded, err := grammar.FromPackages(&packages.Config{BuildFlags: []string{"-tags=jsonschema"}}, p)
	if err != nil {
		t.Fatal(err)
	}
	gp := &goPackage{dir: dir, loadDir: dir, alias: "skgo0", pkg: p}
	a := &app{cfg: Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated"), Package: "generated", Logf: func(string, ...any) {}}, hostDir: root, hostModule: "example.com/app", pkgs: []*goPackage{gp}, grammarPackages: map[string]*grammar.Package{dir: loaded}, typeSets: map[*types.Package]*namedTypes{}}
	a.cfg.generation = &generatedOutput{files: map[string]string{}, parts: map[string][]string{}, owned: map[string]bool{}}
	return a, gp
}
