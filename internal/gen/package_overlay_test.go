package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestReadOnlyPackagesUseCurrentSourcesAndCompilerDependencies(t *testing.T) {
	for _, changedDependency := range []bool{false, true} {
		name := "unchanged dependency"
		if changedDependency {
			name = "changed dependency"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"go.mod":         "module overlayfixture\n\ngo 1.27.1\n",
				"dep/dep.go":     "package dep\ntype Value struct { Text string }\n",
				"route/route.go": "package route\nimport \"overlayfixture/dep\"\ntype Input struct { Value dep.Value }\n",
			}
			for path, text := range files {
				path = filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			overlay := map[string][]byte{
				filepath.Join(dir, "route/route.go"): []byte("package route\nimport \"overlayfixture/dep\"\ntype Input struct { Value dep.Value; Added int }\n"),
			}
			wantDependencyField := "Text"
			if changedDependency {
				wantDependencyField = "Number"
				overlay[filepath.Join(dir, "dep/dep.go")] = []byte("package dep\ntype Value struct { Number int }\n")
			}
			var dependencyParses atomic.Int32
			cfg := &packages.Config{
				Dir: dir, Overlay: overlay,
				Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
				ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
					if filepath.Base(filename) == "dep.go" {
						dependencyParses.Add(1)
					}
					return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
				},
			}
			cleanup, err := preserveDependencyExports(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			loaded, err := packages.Load(cfg, "./route")
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded) != 1 || len(loaded[0].Errors) != 0 {
				t.Fatalf("load: %+v", loaded)
			}
			input := loaded[0].Types.Scope().Lookup("Input").Type().Underlying().(*types.Struct)
			if input.NumFields() != 2 || input.Field(1).Name() != "Added" {
				t.Fatalf("current root was not parsed: %s", input)
			}
			dep := input.Field(0).Type().Underlying().(*types.Struct)
			if dep.NumFields() != 1 || dep.Field(0).Name() != wantDependencyField {
				t.Fatalf("compiler dependency does not match current sources: %s", dep)
			}
			if got := dependencyParses.Load(); got != 0 {
				t.Fatalf("dependency reparsed %d times despite compiler export data", got)
			}
			for path, text := range files {
				got, err := os.ReadFile(filepath.Join(dir, path))
				if err != nil || !bytes.Equal(got, []byte(text)) {
					t.Fatalf("committed %s changed: %v", path, err)
				}
			}
		})
	}
}

func TestFreshPackagesPreserveWorkspaceAssetsAndSourceLocations(t *testing.T) {
	for _, workspace := range []bool{false, true} {
		t.Run(fmt.Sprintf("workspace=%t", workspace), func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "real app")
			alias := filepath.Join(base, "alias app")
			for path, source := range map[string]string{
				"real app/go.mod":                        "module freshfixture\n\ngo 1.27.1\nrequire freshdep v0.0.0\nreplace freshdep => ../dep\n",
				"dep/go.mod":                             "module freshdep\n\ngo 1.27.1\n",
				"dep/dep.go":                             "package dep\ntype Value struct { Text string }\n",
				"real app/route/route.go":                "package route\nimport \"embed\"\n//go:embed assets\nvar Files embed.FS\n",
				"real app/route/assets/nested/value.txt": "literal asset\n",
			} {
				path = filepath.Join(base, path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(dir, alias); err != nil {
				t.Fatal(err)
			}
			before := checkSourceSnapshot(t, dir, filepath.Join(base, "dep"))
			env := append(os.Environ(), "GOWORK=off")
			if workspace {
				work := filepath.Join(base, "go.work")
				if err := os.WriteFile(work, []byte("go 1.27.1\nuse \"./real app\"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				env = append(env, "GOWORK="+work)
			}
			path := filepath.Join(alias, "route/new.go")
			var stdlibParses atomic.Int32
			cfg := &packages.Config{
				Dir: alias, Env: env,
				Mode:    packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
				Overlay: map[string][]byte{path: []byte("package route\nimport (\"freshdep\"; \"net/http\")\ntype Added struct { Value dep.Value; Header http.Header }\nconst DirectiveText = `first\n//line alternate.go:40:5\nlast`\n")},
				ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
					if strings.HasPrefix(filename, runtime.GOROOT()+string(filepath.Separator)) {
						stdlibParses.Add(1)
					}
					return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
				},
			}
			cleanup, err := preserveDependencyExports(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			loaded, err := packages.Load(cfg, "./route")
			if err != nil || len(loaded) != 1 || len(loaded[0].Errors) != 0 {
				t.Fatalf("fresh load: %v, %+v", err, loaded)
			}
			added := loaded[0].Types.Scope().Lookup("Added")
			if added == nil {
				t.Fatal("new declaration missing")
			}
			text := loaded[0].Types.Scope().Lookup("DirectiveText").(*types.Const)
			if got := constant.StringVal(text.Val()); got != "first\n//line alternate.go:40:5\nlast" {
				t.Fatalf("raw string value changed: %q", got)
			}
			shape := added.Type().Underlying().(*types.Struct)
			if shape.NumFields() != 2 || shape.Field(0).Type().Underlying().(*types.Struct).Field(0).Name() != "Text" {
				t.Fatalf("replacement dependency changed: %s", shape)
			}
			if pos := loaded[0].Fset.Position(added.Pos()); pos.Filename != path || pos.Line != 3 {
				t.Fatalf("source location = %s, want %s:3", pos, path)
			}
			if stdlibParses.Load() != 0 {
				t.Fatalf("reparsed %d standard-library files", stdlibParses.Load())
			}
			after := checkSourceSnapshot(t, dir, filepath.Join(base, "dep"))
			if len(before) != len(after) {
				t.Fatalf("changed file count: %d -> %d", len(before), len(after))
			}
			for path, want := range before {
				if !bytes.Equal(after[path], want) {
					t.Fatalf("changed %s", path)
				}
			}
		})
	}
}

func TestFreshPackagesLocateParserAndCompilerErrors(t *testing.T) {
	for _, tc := range []struct{ name, source, want, file, line string }{
		{"parser", "package fresherrors\nfunc broken( {\n", "expected", "new.go", "2"},
		{"compiler", "package fresherrors\nvar Value = missingIdentifier\n", "undefined: missingIdentifier", "new.go", "2"},
		{"relative line directive", "package fresherrors\n//line alternate.go:40:5\nvar Value = missingIdentifier\n", "undefined: missingIdentifier", "alternate.go", "40"},
		{"relative block directive", "package fresherrors\n/*line alternate.go:40:5*/var Value = missingIdentifier\n", "undefined: missingIdentifier", "alternate.go", "40"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fresherrors\n\ngo 1.27.1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "new.go")
			cfg := &packages.Config{Dir: dir, Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax, Overlay: map[string][]byte{path: []byte(tc.source)}}
			cleanup, err := preserveDependencyExports(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			loaded, err := packages.Load(cfg, ".")
			if err != nil || len(loaded) != 1 {
				t.Fatalf("load: %v, %+v", err, loaded)
			}
			found := false
			for _, diagnostic := range loaded[0].Errors {
				if strings.HasPrefix(diagnostic.Pos, filepath.Join(dir, tc.file)+":"+tc.line+":") && strings.Contains(diagnostic.Msg, tc.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing authored %s diagnostic: %v", tc.name, loaded[0].Errors)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("load published source: %v", err)
			}
		})
	}
}

func TestReadOnlyPackagesStillLoadSourceWithoutACommittedCopy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module overlaynew\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "new.go")
	var dependencyParses atomic.Int32
	cfg := &packages.Config{
		Dir: dir, Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax,
		Overlay: map[string][]byte{path: []byte("package overlaynew\nimport \"net/http\"\ntype Added struct { Number int; Header http.Header }\n")},
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			if filepath.Base(filename) != "new.go" {
				dependencyParses.Add(1)
			}
			return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
		},
	}
	cleanup, err := preserveDependencyExports(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	loaded, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || len(loaded[0].Errors) != 0 {
		t.Fatalf("load: %+v", loaded)
	}
	if added := loaded[0].Types.Scope().Lookup("Added"); added == nil || !strings.Contains(added.Type().String(), "Added") {
		t.Fatal("new source was not loaded")
	}
	if got := dependencyParses.Load(); got != 0 {
		t.Fatalf("fresh source reparsed %d dependency files despite compiler exports", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("check wrote an app source file: %v", err)
	}
	view := cfg.Dir
	cleanup()
	if _, err := os.Stat(view); !os.IsNotExist(err) {
		t.Fatalf("package view survived cleanup: %v", err)
	}
}

func TestReadOnlyPackagesResolveAliasedRootsAndDependencies(t *testing.T) {
	for _, name := range []string{"real-dir-alias-overlay", "alias-dir-real-overlay", "file-alias-overlay"} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "real")
			alias := filepath.Join(base, "alias")
			for _, folder := range []string{dir, filepath.Join(dir, "dep"), filepath.Join(dir, "route")} {
				if err := os.MkdirAll(folder, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(dir, alias); err != nil {
				t.Fatal(err)
			}
			for path, text := range map[string]string{"go.mod": "module aliasfixture\n\ngo 1.27.1\n", "dep/dep.go": "package dep\ntype Value struct { Text string }\n", "route/route.go": "package route\nimport \"aliasfixture/dep\"\ntype Input struct { Value dep.Value }\n"} {
				if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			working, overlayRoot := dir, alias
			if name == "alias-dir-real-overlay" {
				working, overlayRoot = alias, dir
			}
			depPath := filepath.Join(overlayRoot, "dep/dep.go")
			routePath := filepath.Join(overlayRoot, "route/route.go")
			if name == "file-alias-overlay" {
				depPath = filepath.Join(base, "dep-alias.go")
				routePath = filepath.Join(base, "route-alias.go")
				if err := os.Symlink(filepath.Join(dir, "dep/dep.go"), depPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(dir, "route/route.go"), routePath); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &packages.Config{Dir: working, Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedImports, Overlay: map[string][]byte{routePath: []byte("package route\nimport \"aliasfixture/dep\"\ntype Input struct { Value dep.Value; Added int }\n"), depPath: []byte("package dep\ntype Value struct { Number int }\n")}}
			cleanup, err := preserveDependencyExports(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			loaded, err := packages.Load(cfg, "./route")
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded) != 1 || len(loaded[0].Errors) != 0 {
				t.Fatalf("load: %+v", loaded)
			}
			input := loaded[0].Types.Scope().Lookup("Input").Type().Underlying().(*types.Struct)
			if input.NumFields() != 2 || input.Field(1).Name() != "Added" {
				t.Fatalf("root stale: %s", input)
			}
			dep := input.Field(0).Type().Underlying().(*types.Struct)
			if dep.NumFields() != 1 || dep.Field(0).Name() != "Number" {
				t.Fatalf("dependency stale: %s", dep)
			}
		})
	}
}

func checkSourceSnapshot(t *testing.T, roots ...string) map[string][]byte {
	t.Helper()
	snapshot := map[string][]byte{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
			}
			b, err := os.ReadFile(path)
			if err == nil {
				snapshot[path] = b
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}

func TestReadOnlyCheckReportsAuthoredSourceErrorsWithoutWrites(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"parser", "\nfunc checkBroken( {\n", "expected"},
		{"compiler", "\nvar _ = checkMissingIdentifier\n", "undefined: checkMissingIdentifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := sandboxExample(t)
			web := filepath.Join(app, "web")
			out := filepath.Join(app, "internal", "skgo")
			path := filepath.Join(web, "src", "routes", "todos", "todos.remote.go")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			planted := append(append([]byte(nil), original...), []byte(tc.source)...)
			if err := os.WriteFile(path, planted, 0600); err != nil {
				t.Fatal(err)
			}
			before := checkSourceSnapshot(t, filepath.Join(web, "src"), out)
			err = Check(fixtureConfig(Config{Web: web, Out: out}))
			if err == nil || !strings.Contains(err.Error(), path+":") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("missing authored %s diagnostic: %v", tc.name, err)
			}
			t.Log(err)
			after := checkSourceSnapshot(t, filepath.Join(web, "src"), out)
			if len(before) != len(after) {
				t.Fatalf("Check changed file count: %d -> %d", len(before), len(after))
			}
			for path, want := range before {
				if !bytes.Equal(after[path], want) {
					t.Fatalf("Check changed %s", path)
				}
			}
		})
	}
}

func TestReadOnlyPackagesPreserveExternalDriverOverlays(t *testing.T) {
	binDir := t.TempDir()
	source := filepath.Join(binDir, "driver.go")
	const driverSource = `package main
import ("encoding/json"; "fmt"; "os")
func main() {
 var request struct { Overlay map[string][]byte }
 if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil { panic(err) }
 if len(request.Overlay) != 1 { fmt.Fprintln(os.Stderr, "driver did not receive current source overlay"); os.Exit(1) }
 fmt.Println("{\"NotHandled\":true}")
}`
	if err := os.WriteFile(source, []byte(driverSource), 0600); err != nil {
		t.Fatal(err)
	}
	driverPath := filepath.Join(binDir, "gopackagesdriver")
	build := exec.Command("go", "build", "-o", driverPath, source)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build package driver: %v\n%s", err, out)
	}
	for _, explicit := range []bool{false, true} {
		name := "auto-discovered"
		if explicit {
			name = "configured"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GOPACKAGESDRIVER", "")
			if explicit {
				t.Setenv("GOPACKAGESDRIVER", driverPath)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module externalfixture\n\ngo 1.27.1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "route.go")
			if err := os.WriteFile(path, []byte("package externalfixture\ntype OldSource int\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := &packages.Config{Dir: dir, Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax, Overlay: map[string][]byte{path: []byte("package externalfixture\ntype NewSource int\n")}}
			cleanup, err := preserveDependencyExports(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			loaded, err := packages.Load(cfg, ".")
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded) != 1 || len(loaded[0].Errors) != 0 {
				t.Fatalf("driver load: %+v", loaded)
			}
			if loaded[0].Types.Scope().Lookup("NewSource") == nil || loaded[0].Types.Scope().Lookup("OldSource") != nil {
				t.Fatal("external driver lost current authored source")
			}
		})
	}
}
