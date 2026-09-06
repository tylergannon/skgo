package gen

import (
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rule that decides where a type's declaration is written, on its own.
//
// `declare` resolves whichever package *defines* a wire type. For the app's own
// packages the declaration goes beside that source, which is where a developer
// would look for it. For any type declared outside the app — a dependency in
// the module cache, a shared internal module, another repository — that
// directory belongs to somebody else and, for any real consumer, is read-only;
// so the declaration comes to the app instead, exactly as `typesDirFor` already
// brings a foreign package's types.ts to `src/lib/skgo/`.
//
// The rule is therefore about the directory, decided before anything is
// written, and never about whether that directory happens to be writable — a
// writable dependency is the case that hid this bug, not the case that excuses
// it.

func TestOnlyTheAppsOwnPackagesGetTheirDeclarationInPlace(t *testing.T) {
	root := t.TempDir()
	mkdir := func(rel string) string {
		t.Helper()
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	app := mkdir("app")
	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"the app's own root", app, true},
		{"a package in the app", mkdir("app/businesslogic"), true},
		// The route tree carries a `go.mod` of its own — the boundary that
		// stops `go build ./...` walking into `[id]` — so a package the
		// developer authored reports a module path that is not the app's.
		// Comparing module paths would relocate the developer's own source;
		// containment is what actually answers the question.
		{"a route package behind skgo's own go.mod boundary", mkdir("app/web/src/routes/items"), true},
		{"a dependency in the module cache", mkdir("gopath/pkg/mod/example.com/wire@v1.2.3"), false},
		{"a sibling module in the same checkout", mkdir("wire"), false},
		{"a parent of the app", root, false},
		// `app2` shares a prefix with `app` but is not inside it. A string
		// prefix test says otherwise.
		{"a directory whose name merely starts with the app's", mkdir("app2/data"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinTree(app, tc.dir); got != tc.want {
				t.Errorf("withinTree(%s, %s) = %v, want %v", app, tc.dir, got, tc.want)
			}
		})
	}
}

// `go list -f {{.Dir}}` reports a path with every symlink resolved, while the
// app's own root may still hold one — /var against /private/var is the
// everyday macOS case, and it would relocate every package in the app.
func TestOwnershipSurvivesASymlinkedPath(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(real, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("this filesystem does not do symlinks: %v", err)
	}

	if !withinTree(link, filepath.Join(real, "pkg")) {
		t.Error("a package under the app's real path was treated as foreign when the app was reached through a symlink")
	}
	if !withinTree(real, filepath.Join(link, "pkg")) {
		t.Error("a package reached through a symlink into the app was treated as foreign")
	}
	if withinTree(link, t.TempDir()) {
		t.Error("an unrelated directory was treated as the app's own")
	}
}

// Where a relocated declaration lands. It has to be inside the app's own
// module — that is what makes it compilable, nameable by Go, and a directory
// `go:embed` can reach — and it has to be addressed by the foreign package's
// import path, because package names are short, not unique, and not the app's
// to change.
func TestARelocatedDeclarationLandsInTheAppsOwnGeneratedTree(t *testing.T) {
	a := &app{
		cfg:        Config{Out: filepath.FromSlash("/app/generated")},
		hostDir:    filepath.FromSlash("/app"),
		hostModule: "example.com/app",
	}

	first, err := a.declarationDirFor(types.NewPackage("example.com/wire", "wire"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.FromSlash("/app/generated/wiretypes/example.com/wire"); first != want {
		t.Fatalf("declarationDirFor = %q, want %q", first, want)
	}
	if !withinTree(a.hostDir, first) {
		t.Fatalf("%s is not inside the app", first)
	}
	// It must not fall inside the route tree, whose own go.mod would put it in
	// a different module from the bindings that use it.
	if withinTree(filepath.FromSlash("/app/web/src/routes"), first) {
		t.Fatalf("%s is behind the route tree's module boundary", first)
	}

	second, err := a.declarationDirFor(types.NewPackage("example.com/other/wire", "wire"))
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatalf("two packages both called wire were given the same directory %q", first)
	}

	// A path Go cannot name is refused rather than written into a directory
	// the toolchain will then refuse to compile.
	if _, err := a.declarationDirFor(types.NewPackage("example.com/wire/[id]", "wire")); err == nil {
		t.Fatal("a package whose import path Go cannot name was accepted")
	} else if !strings.Contains(err.Error(), "example.com/wire/[id]") {
		t.Fatalf("the refusal does not name the package:\n%v", err)
	}
}
