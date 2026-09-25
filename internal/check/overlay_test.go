package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRouteOverlayUsesAuthoredBytesWithoutChangingCopy(t *testing.T) {
	root := t.TempDir()
	authored := filepath.Join(root, "web", "src", "routes", "todos", "todos.remote.go")
	copy := filepath.Join(root, "internal", "skgo", "links", "todos", "todos.remote.go")
	for _, p := range []string{authored, copy} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(authored, []byte("package todos\nvar Current=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := []byte("package todos\nvar Current=0\n")
	if err := os.WriteFile(copy, old, 0644); err != nil {
		t.Fatal(err)
	}
	routes := routeInventory{Links: []routeLink{{Link: "internal/skgo/links/todos", Target: "web/src/routes/todos"}}}
	path, cleanup, err := makeOverlay(root, routes)
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Replace map[string]string }
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if v.Replace[copy] != authored {
		t.Fatalf("overlay=%v", v.Replace)
	}
	if got := authoredPath(root, routes, copy); got != "web/src/routes/todos/todos.remote.go" {
		t.Fatalf("mapped path=%q", got)
	}
	message := authoredMessage(root, routes, "skgo: "+copy+":2:5: unsupported wire type")
	d := diagnosticFromError("skgo", "SKGO007", root, message)
	if d.Location == nil || d.Location.File != "web/src/routes/todos/todos.remote.go" || d.Location.Line != 2 {
		t.Fatalf("authored diagnostic=%+v", d)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary overlay remains: %v", err)
	}
	if b, err := os.ReadFile(copy); err != nil || string(b) != string(old) {
		t.Fatalf("generated copy changed: %q, %v", b, err)
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
