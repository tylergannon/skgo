package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// fakeApp lays out a vite root inside a Go module, with Go files in the route
// directories a SvelteKit developer actually writes.
func fakeApp(t *testing.T, dirs ...string) Config {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.27.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(root, "web")
	for _, dir := range dirs {
		full := filepath.Join(web, filepath.FromSlash(dir))
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "data.remote.go"), []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	return fixtureConfig(Config{Web: web, Out: filepath.Join(root, "generated"), Logf: func(string, ...any) {}})
}

func linksFor(t *testing.T, cfg Config) *routeLinks {
	t.Helper()
	hostDir, hostModule, err := moduleOf(cfg.Out)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := newRouteLinks(cfg, hostDir, hostModule)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// TestEveryRouteDirectoryBecomesAnImportableGoPackage is the whole point of the
// link tree: a developer puts a `.remote.go` wherever the route lives, brackets
// and parentheses included, and Go gets an address it can spell.
func TestEveryRouteDirectoryBecomesAnImportableGoPackage(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t,
		"src/routes",
		"src/routes/todos",
		"src/routes/todos/[id]",
		"src/routes/(marketing)/pricing",
		"src/routes/docs/[...rest]",
	)
	tree := linksFor(t, cfg)
	if err := tree.sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	a := &app{cfg: cfg, hostDir: tree.hostDir, hostModule: tree.hostModule, links: tree}
	for _, rel := range []string{
		"src/routes",
		"src/routes/todos",
		"src/routes/todos/[id]",
		"src/routes/(marketing)/pricing",
		"src/routes/docs/[...rest]",
	} {
		dir := filepath.Join(cfg.Web, filepath.FromSlash(rel))
		path, err := a.importPath(dir)
		if err != nil {
			t.Fatalf("importPath(%s): %v", rel, err)
		}
		want := "example.com/app/generated/_links/" + encodeLinkName(rel)
		if path != want {
			t.Fatalf("importPath(%s) = %q, want %q", rel, path, want)
		}
		linkDir := filepath.Join(cfg.Out, linkRootName, encodeLinkName(rel))
		generated, err := os.ReadFile(filepath.Join(linkDir, "data.remote.go"))
		if err != nil || string(generated) != "package p\n" {
			t.Fatalf("%s does not contain the authored source: %q, %v", linkDir, generated, err)
		}
		if got := tree.authoredDir(linkDir); got != dir {
			t.Fatalf("authoredDir(%s) = %s, want %s", linkDir, got, dir)
		}
		file := filepath.Join(linkDir, "data.remote.go")
		if got := tree.authoredPath(file); got != filepath.Join(dir, "data.remote.go") {
			t.Fatalf("authoredPath(%s) = %s", file, got)
		}
	}
}

// TestTheModuleIgnoresTheRouteTree checks the directive whose absence turns
// `go build ./...` into `invalid char '['`, and that adding it keeps what the
// author wrote.
func TestTheModuleIgnoresTheRouteTree(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/todos/[id]")
	goMod := filepath.Join(filepath.Dir(cfg.Web), "go.mod")
	authored := "module example.com/app\n\ngo 1.27.1\n\n// The author's own note.\nignore ./web/node_modules\n"
	if err := os.WriteFile(goMod, []byte(authored), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	first, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	f, err := modfile.Parse(goMod, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ignored []string
	for _, ignore := range f.Ignore {
		ignored = append(ignored, ignore.Path)
	}
	if strings.Join(ignored, " ") != "./web/node_modules ./web/src/routes" {
		t.Fatalf("ignore directives = %v, want the author's and ./web/src/routes:\n%s", ignored, first)
	}
	if !strings.Contains(string(first), "// The author's own note.") || f.Module.Mod.Path != "example.com/app" {
		t.Fatalf("adding the directive lost what the author wrote:\n%s", first)
	}
	if _, err := os.Stat(filepath.Join(cfg.Web, "src", "routes", "go.mod")); !os.IsNotExist(err) {
		t.Fatalf("a module boundary was written into the route tree: %v", err)
	}
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	if second, err := os.ReadFile(goMod); err != nil || string(second) != string(first) {
		t.Fatalf("a second run rewrote go.mod:\n%s", second)
	}
}

// TestAnAuthoredBoundaryIsLeftAlone: skgo never deletes a route-tree go.mod
// it did not write. Such a file makes the route root its own module, so a
// route root holding Go files is refused by name rather than by Go's
// "does not contain package".
func TestAnAuthoredBoundaryIsLeftAlone(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/todos/[id]")
	path := filepath.Join(cfg.Web, "src", "routes", "go.mod")
	authored := "module example.com/routes\n\ngo 1.27.1\n"
	if err := os.WriteFile(path, []byte(authored), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Web, "src", "routes", "root.go"), []byte("package routes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linksFor(t, cfg).sync(); err == nil || !strings.Contains(err.Error(), "web/src/routes/go.mod makes the route root its own module") {
		t.Fatalf("sync with an authored boundary over route-root Go = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != authored {
		t.Fatalf("skgo rewrote a boundary it did not write:\n%s", raw)
	}
}

// TestTheLinkTreeRegeneratesDeterministically is what makes the tree
// disposable: two runs produce the same bytes, and a route directory that goes
// away takes its link with it.
func TestTheLinkTreeRegeneratesDeterministically(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/todos", "src/routes/todos/[id]")
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	inventoryPath := filepath.Join(cfg.Out, inventoryName)
	first, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("the inventory changed on a second run:\n%s\n---\n%s", first, second)
	}

	stale := filepath.Join(cfg.Out, linkRootName, encodeLinkName("src/routes/todos/[id]"))
	if _, err := os.Lstat(stale); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(cfg.Web, "src", "routes", "todos", "[id]")); err != nil {
		t.Fatal(err)
	}
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stale); err == nil {
		t.Fatal("the link to a deleted route directory survived regeneration")
	}
}

// TestRemovingTheLastGoFileRemovesEveryLink: skgo tidies up after itself
// rather than leaving links in an app that no longer has one Go file under its
// routes.
func TestRemovingTheLastGoFileRemovesEveryLink(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/todos/[id]")
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cfg.Web, "src", "routes", "todos", "[id]", "data.remote.go")); err != nil {
		t.Fatal(err)
	}
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(cfg.Out, linkRootName))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("links left behind: %v", entries)
	}
}

// TestPruningRefusesToDeleteSomethingItDidNotCreate keeps a misconfigured
// output directory from costing anybody their source.
func TestPruningRefusesToDeleteSomethingItDidNotCreate(t *testing.T) {
	t.Parallel()
	cfg := fakeApp(t, "src/routes/todos")
	tree := linksFor(t, cfg)
	if err := tree.sync(); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(cfg.Out, linkRootName, "notes.go")
	if err := os.WriteFile(stray, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := tree.sync()
	if err == nil {
		t.Fatal("sync deleted a regular file it had not created")
	}
	if !strings.Contains(err.Error(), "will not delete") {
		t.Fatalf("error = %q, want it to say what it refused", err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatalf("the file was removed anyway: %v", err)
	}
}

// TestLinkNamesAreTheDocumentedEncoding pins the naming scheme, because the
// name is what the generated bindings import: change it silently and every
// checked-in bindings file stops compiling.
func TestLinkNamesAreTheDocumentedEncoding(t *testing.T) {
	t.Parallel()
	for rel, want := range map[string]string{
		"src/routes":            "onzggl3sn52xizlt",
		"src/routes/todos":      "onzggl3sn52xizltf52g6zdpom",
		"src/routes/items/[id]": "onzggl3sn52xizltf5uxizlnomxvw2lelu",
	} {
		if got := encodeLinkName(rel); got != want {
			t.Fatalf("encodeLinkName(%q) = %q, want %q", rel, got, want)
		}
	}
}
