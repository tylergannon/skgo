package gen

import (
	"os"
	"path/filepath"
	"testing"
)

// The rule that decides whether a wire type is the app's own, on its own.
//
// `declare` resolves whichever package *defines* a wire type. For the app's own
// packages polytype loads that package and its types.ts goes beside the
// source, which is where a developer would look for it. For any type declared
// outside the app — a dependency in the module cache, a shared internal
// module, another repository — that directory belongs to somebody else, so
// polytype is pointed at the app package importing it instead, and
// `typesDirFor` brings its types.ts to `src/lib/skgo/` under its import path.
//
// The rule is therefore about the directory, decided before anything is
// loaded, and never about whether that directory happens to be writable — a
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
