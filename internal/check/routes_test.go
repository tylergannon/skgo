package check

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRouteLinkDiagnosticsNameTheAuthoredFile(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "internal", "skgo", "links", "todos", "todos.remote.go")
	routes := routeInventory{Links: []routeLink{{Link: "internal/skgo/links/todos", Target: "web/src/routes/todos"}}}
	if got := authoredPath(root, routes, link); got != "web/src/routes/todos/todos.remote.go" {
		t.Fatalf("mapped path=%q", got)
	}
	message := authoredMessage(root, routes, "skgo: "+link+":2:5: unsupported wire type")
	d := diagnosticFromError("skgo", "SKGO007", root, message)
	if d.Location == nil || d.Location.File != "web/src/routes/todos/todos.remote.go" || d.Location.Line != 2 {
		t.Fatalf("authored diagnostic=%+v", d)
	}
	if got := goPackages(routes); len(got) != 2 || got[0] != "./..." || got[1] != "./internal/skgo/links/todos" {
		t.Fatalf("checked packages=%q", got)
	}
}

func TestAuthoredPathThroughSymlinkedRoot(t *testing.T) {
	physical := t.TempDir()
	alias := filepath.Join(t.TempDir(), "app")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(physical, "src", "routes", "todos.remote.go")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package routes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := authoredPath(alias, routeInventory{}, path); got != "src/routes/todos.remote.go" {
		t.Fatalf("authored path = %q", got)
	}
}
