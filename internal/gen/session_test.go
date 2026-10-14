package gen

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tylergannon/polytype/goload"
	"golang.org/x/tools/go/packages"
)

func TestGenerationCacheLeavesSessionUnusedOnHit(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	// Publish with the ordinary loader; the optional session cannot change the
	// persistent key or eligibility for the next invocation.
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	binding := filepath.Join(cfg.Out, generatedGoFile)
	expected, err := os.ReadFile(binding)
	if err != nil {
		t.Fatal(err)
	}
	assertCacheFile(t, filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"), `"name": string`)
	cfg.Session = new(goload.Session)
	for _, missing := range []bool{false, true} {
		if missing {
			if err := os.Remove(binding); err != nil {
				t.Fatal(err)
			}
		}
		if err := Run(cfg); err != nil {
			t.Fatal(err)
		}
		if stats := cfg.Session.Stats(); stats != (goload.Stats{}) {
			t.Fatalf("persistent hit touched loader session: %+v", stats)
		}
		actual, err := os.ReadFile(binding)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("binding was not restored exactly: %v", err)
		}
	}
	// A dependency edit must traverse the session and change the actual public
	// contract, even after both warm replay and missing-file restoration.
	cacheWrite(t, filepath.Join(root, "wire/wire.go"), "package wire\ntype Thing struct { Changed bool `json:\"changed\"` }\n")
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if stats := cfg.Session.Stats(); stats.PackageChecks == 0 || stats.FallbackLoads != 0 {
		t.Fatalf("changed dependency did not use shared analysis: %+v", stats)
	}
	path := filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts")
	actual, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(actual), `"changed": boolean`) || strings.Contains(string(actual), `"name": string`) {
		t.Fatalf("stale dependency projection: %s %v", actual, err)
	}
}

func TestSessionFallbackPreservesDependencyExports(t *testing.T) {
	root := t.TempDir()
	dep := t.TempDir()
	cacheWrite(t, filepath.Join(dep, "go.mod"), "module fallbackdep\n\ngo 1.27.1\n")
	depPath := filepath.Join(dep, "value.go")
	const depSource = "package fallbackdep\nconst Number = 7\n"
	cacheWrite(t, depPath, depSource)
	cacheWrite(t, filepath.Join(root, "go.mod"), "module sessionfallback\n\ngo 1.27.1\nrequire fallbackdep v0.0.0\nreplace fallbackdep => "+filepath.ToSlash(dep)+"\n")
	path := filepath.Join(root, "types.go")
	const original = "package fixture\nimport \"net/http\"\ntype Value struct { Handler http.Handler }\n"
	cacheWrite(t, path, original)
	var mu sync.Mutex
	parsed := map[string]bool{}
	session := new(goload.Session)
	cfg := Config{Session: session, sourceView: &generationSourceView{sources: map[string][]byte{depPath: []byte(depSource), path: []byte(original)}}}
	load := &packages.Config{
		Dir:     root,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Overlay: map[string][]byte{filepath.Join(root, "generated.go"): []byte("package fixture\nimport \"fallbackdep\"\nconst Restored = 10 + fallbackdep.Number\n")},
		ParseFile: func(fset *token.FileSet, name string, source []byte) (*ast.File, error) {
			mu.Lock()
			parsed[name] = true
			mu.Unlock()
			return parser.ParseFile(fset, name, source, parser.ParseComments|parser.AllErrors)
		},
	}
	roots, err := cfg.loadPackages(load, "sessionfallback")
	if err != nil || len(roots) != 1 || len(roots[0].Errors) != 0 {
		t.Fatalf("fallback load: %v %v", roots, err)
	}
	if got, ok := roots[0].Types.Scope().Lookup("Restored").(*types.Const); !ok || got.Val().ExactString() != "17" {
		t.Fatalf("virtual generated declaration missing: %v", got)
	}
	if session.Stats().FallbackLoads != 1 || len(parsed) != 2 {
		t.Fatalf("fallback reparsed dependencies: stats=%+v parsed=%d", session.Stats(), len(parsed))
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != original {
		t.Fatalf("fallback changed authored source: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "generated.go")); !os.IsNotExist(err) {
		t.Fatalf("fallback published generated input: %v", err)
	}
}

func TestGenerationCapturesEmptyEmbedDirectoryMembership(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	loads := 0
	cfg.OnPackageLoad = func(*packages.Config, []string) { loads++ }
	cacheWrite(t, filepath.Join(root, "wire/wire.go"), "package wire\nimport \"embed\"\n//go:embed data/*\nvar Files embed.FS\ntype Thing struct { Name string `json:\"name\"` }\n")
	cacheWrite(t, filepath.Join(root, "wire/data/one.txt"), "one")
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	assertCacheFile(t, filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"), `"name": string`)
	if loads == 0 {
		t.Fatal("initial generation did not analyze source")
	}
	loads = 0
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if loads != 0 {
		t.Fatal("unchanged fixture did not use persistent cache")
	}
	empty := filepath.Join(root, "wire/data/empty")
	if err := os.MkdirAll(empty, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "contains no embeddable files") {
		t.Fatalf("new invalid embed directory reused stale generation: %v", err)
	}
	if err := os.Remove(empty); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCapturesOnlyRequestedParserSources(t *testing.T) {
	root := t.TempDir()
	var err error
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, filepath.Join(root, "go.mod"), "module fixture.test/captured\n\ngo 1.27.1\n")
	path := filepath.Join(root, "root.go")
	cacheWrite(t, path, "package captured\nthis is not valid Go\n")
	cacheWrite(t, filepath.Join(root, "dep/dep.go"), "package dep\nconst Number = 7\n")
	source := []byte("package captured\nimport \"fixture.test/captured/dep\"\nconst Answer = dep.Number + 10\n")
	phase := filepath.Join(root, "phase.go")
	unrelated := filepath.Join(root, "unrelated/bad.go")
	cfg := Config{Session: new(goload.Session), sourceView: &generationSourceView{
		sources:     map[string][]byte{path: source, unrelated: []byte("invalid unrelated captured source")},
		moduleRoots: map[string]string{"fixture.test/captured": root},
	}}
	load := &packages.Config{Dir: root, Mode: packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax,
		Overlay: map[string][]byte{phase: []byte("package captured\nconst Phase = 3\n")},
	}
	// The compiler reads dependencies from the already captured private tree.
	// Only selected roots need a second representation for the session parser.
	cfg.sessionSources(load, "fixture.test/captured")
	if len(load.Overlay) != 2 || !bytes.Equal(load.Overlay[path], source) || load.Overlay[unrelated] != nil {
		t.Fatalf("unrelated capture serialized: %v", load.Overlay)
	}
	roots, err := cfg.loadPackages(load, "fixture.test/captured")
	if err != nil || len(roots) != 1 || len(roots[0].Errors) != 0 {
		t.Fatalf("captured load: %v %v", roots, err)
	}
	for name, want := range map[string]string{"Answer": "17", "Phase": "3"} {
		got, ok := roots[0].Types.Scope().Lookup(name).(*types.Const)
		if !ok || got.Val().ExactString() != want {
			t.Fatalf("%s=%v, want %s", name, got, want)
		}
	}
	if stats := cfg.Session.Stats(); stats.FileReads != 0 || stats.FallbackLoads != 0 {
		t.Fatalf("captured root reread: %+v", stats)
	}
	// Dependency phase overlays still control Go's compiler export input.
	load.Overlay[filepath.Join(root, "dep/dep.go")] = []byte("package dep\nconst Number = 13\n")
	roots, err = cfg.loadPackages(load, "fixture.test/captured")
	if err != nil || len(roots) != 1 || len(roots[0].Errors) != 0 {
		t.Fatalf("dependency phase load: %v %v", roots, err)
	}
	if got := roots[0].Types.Scope().Lookup("Answer").(*types.Const).Val().ExactString(); got != "23" {
		t.Fatalf("dependency override lost: %s", got)
	}
	// An explicit phase overlay overrides even a captured root file.
	load.Overlay[path] = []byte("package captured\nconst Answer = 29\n")
	roots, err = cfg.loadPackages(load, "fixture.test/captured")
	if err != nil || len(roots) != 1 || len(roots[0].Errors) != 0 {
		t.Fatalf("phase load: %v %v", roots, err)
	}
	if got := roots[0].Types.Scope().Lookup("Answer").(*types.Const).Val().ExactString(); got != "29" {
		t.Fatalf("phase override lost: %s", got)
	}
}
