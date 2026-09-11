package gen

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
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
	return Config{Web: web, Out: filepath.Join(root, "generated"), Logf: func(string, ...any) {}}
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
		want := "example.com/app/generated/links/" + encodeLinkName(rel)
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

// TestGeneratedRoutePackagesContainFilesNotSymlinks proves the generated tree
// survives a Go module archive, which never includes symlink contents.
func TestGeneratedRoutePackagesContainFilesNotSymlinks(t *testing.T) {
	cfg := fakeApp(t, "src/routes", "src/routes/todos")
	tree := linksFor(t, cfg)
	if err := tree.sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	linkDir := filepath.Join(cfg.Out, linkRootName, encodeLinkName("src/routes"))
	fi, err := os.Lstat(linkDir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Fatal("the generated route package is not a real directory")
	}
	entries, err := os.ReadDir(linkDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || e.IsDir() {
			t.Fatalf("%s is not a regular file", e.Name())
		}
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "data.remote.go" {
		t.Fatalf("route root link holds %v, want just the Go source", names)
	}
	if _, err := os.Lstat(filepath.Join(linkDir, "go.mod")); err == nil {
		t.Fatal("the boundary go.mod was linked into the package")
	}
}

func TestGeneratedRoutePackagesSurviveAModuleArchive(t *testing.T) {
	cfg := fakeApp(t, "src/routes/todos/[id]")
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := modzip.CreateFromDir(&archive, module.Version{Path: "example.com/app", Version: "v1.0.0"}, filepath.Dir(cfg.Web)); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	want := "example.com/app@v1.0.0/generated/links/" + encodeLinkName("src/routes/todos/[id]") + "/data.remote.go"
	for _, file := range zr.File {
		if file.Name == want {
			return
		}
	}
	t.Fatalf("module archive omitted generated route source %s", want)
}

// TestTheBoundaryStopsTheParentModuleWalkingIn checks the file whose absence
// turns `go build ./...` into `invalid char '['`.
func TestTheBoundaryStopsTheParentModuleWalkingIn(t *testing.T) {
	cfg := fakeApp(t, "src/routes/todos/[id]")
	tree := linksFor(t, cfg)
	if err := tree.sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(cfg.Web, "src", "routes", "go.mod"))
	if err != nil {
		t.Fatalf("no module boundary: %v", err)
	}
	if !strings.HasPrefix(string(raw), boundaryMarker) {
		t.Fatalf("the boundary is not marked as generated:\n%s", raw)
	}
	if !strings.Contains(string(raw), "module example.com/app/web/src/routes\n") {
		t.Fatalf("the boundary declares the wrong module:\n%s", raw)
	}
	if tree.boundary == nil || !tree.boundary.Owned {
		t.Fatalf("boundary ownership = %+v, want owned", tree.boundary)
	}
}

// TestAnAuthoredBoundaryIsLeftAlone: an app that wants its route tree to be a
// real module of its own keeps it.
func TestAnAuthoredBoundaryIsLeftAlone(t *testing.T) {
	cfg := fakeApp(t, "src/routes/todos/[id]")
	path := filepath.Join(cfg.Web, "src", "routes", "go.mod")
	authored := "module example.com/routes\n\ngo 1.27.1\n"
	if err := os.WriteFile(path, []byte(authored), 0o644); err != nil {
		t.Fatal(err)
	}

	tree := linksFor(t, cfg)
	if err := tree.sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != authored {
		t.Fatalf("skgo rewrote a boundary it did not write:\n%s", raw)
	}
	if tree.boundary == nil || tree.boundary.Owned {
		t.Fatalf("boundary ownership = %+v, want not owned", tree.boundary)
	}
}

// TestTheLinkTreeRegeneratesDeterministically is what makes the tree
// disposable: two runs produce the same bytes, and a route directory that goes
// away takes its link with it.
func TestTheLinkTreeRegeneratesDeterministically(t *testing.T) {
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

// TestRemovingTheLastGoFileRemovesTheBoundary: skgo tidies up after itself
// rather than leaving a module boundary in an app that no longer has one Go
// file under its routes.
func TestRemovingTheLastGoFileRemovesTheBoundary(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(cfg.Web, "src", "routes", "go.mod")); err == nil {
		t.Fatal("the boundary outlived the Go files it existed for")
	}
	entries, err := os.ReadDir(filepath.Join(cfg.Out, linkRootName))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("links left behind: %v", entries)
	}
}

// TestTheRoutePackageHoldsOnlyGeneratedCopies: the route package is a directory
// skgo rebuilds from the route tree on every run, so whatever an earlier run
// left in it — polytype's CLI output once lived there, because `go:embed`
// cannot follow a symlink — is removed rather than compiled into the package.
func TestTheRoutePackageHoldsOnlyGeneratedCopies(t *testing.T) {
	cfg := fakeApp(t, "src/routes")
	tree := linksFor(t, cfg)
	if err := tree.sync(); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(cfg.Out, linkRootName, encodeLinkName("src/routes"))
	if err := os.WriteFile(filepath.Join(linkDir, "jsonschema_gen.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(linkDir, "jsonschema"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linkDir, "jsonschema", "Item.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(linkDir, "jsonschema_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("a stray Go file survived in the route root's link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(linkDir, "jsonschema")); !os.IsNotExist(err) {
		t.Fatalf("a stray directory survived in the route root's link: %v", err)
	}
	generated, err := os.ReadFile(filepath.Join(linkDir, "data.remote.go"))
	if err != nil || string(generated) != "package p\n" {
		t.Fatalf("the authored source was not restored: %q, %v", generated, err)
	}
}

// TestAStaleLinkInTheRouteRootGoes: a file the developer deleted must not keep
// a dangling link, which would stop the package compiling altogether.
func TestAStaleLinkInTheRouteRootGoes(t *testing.T) {
	cfg := fakeApp(t, "src/routes")
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(cfg.Out, linkRootName, encodeLinkName("src/routes"))
	extra := filepath.Join(cfg.Web, "src", "routes", "helper.go")
	if err := os.WriteFile(extra, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(linkDir, "helper.go")); err != nil {
		t.Fatalf("a new Go file was not linked: %v", err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(linkDir, "helper.go")); err == nil {
		t.Fatal("a link to a deleted file survived, leaving the package uncompilable")
	}
}

// TestPruningRefusesToDeleteSomethingItDidNotCreate keeps a misconfigured
// output directory from costing anybody their source.
func TestPruningRefusesToDeleteSomethingItDidNotCreate(t *testing.T) {
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
