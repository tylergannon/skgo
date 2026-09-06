package gen

import (
	"os"
	"path/filepath"
	"testing"
)

// The rule that decides where the generator may write, on its own.
//
// `declare` resolves whichever package *defines* a wire type and writes
// `skgo_polytype_gen.go` — and then polytype's `jsonschema/` — into that
// package's own source directory. For any type declared outside the app that
// directory belongs to somebody else: the module cache for a dependency, a
// shared internal module, another repository. skgo's own module was only the
// instance this repo could see, because a go.work member is writable and the
// scribble left no mark.
//
// The TypeScript half of the same problem is already solved by relocation —
// `typesDirFor` sends any package outside the vite root to
// `src/lib/skgo/<pkg>` instead of writing beside foreign source. The Go half
// cannot copy that answer: polytype's registration is `func (T) Schema()
// json.RawMessage`, a method on T, and Go does not allow a method on a type
// declared in another package. There is no other directory the file could go
// in. So the Go half refuses where the TypeScript half relocates.
//
// The rule is therefore about the directory, decided before anything is
// written, and never about whether that directory happens to be writable —
// a writable dependency is the case that hid this bug, not the case that
// excuses it.

func TestTheGeneratorMayOnlyWriteInsideTheAppsOwnTree(t *testing.T) {
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
		// Comparing module paths would refuse the developer's own source;
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
// everyday macOS case, and it would refuse every package in the app.
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
		t.Error("a package under the app's real path was refused when the app was reached through a symlink")
	}
	if !withinTree(real, filepath.Join(link, "pkg")) {
		t.Error("a package reached through a symlink into the app was refused")
	}
	if withinTree(link, t.TempDir()) {
		t.Error("an unrelated directory was accepted")
	}
}
