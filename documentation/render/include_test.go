package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncludeAllowsRestParamDirectories(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "routes", "[...rest]")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package x\n\n// docs: handler\nfunc get() {}\n// docs: end handler\n"
	if err := os.WriteFile(filepath.Join(dir, "server.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := includeRegion(root, "routes/[...rest]/server.go", "handler")
	if err != nil {
		t.Fatal(err)
	}
	if got != "func get() {}" {
		t.Fatalf("got %q", got)
	}
}

func TestIncludeRejectsEscapingPaths(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../outside", "a/../../outside", "/abs", ".."} {
		_, err := includeRegion(root, rel, "")
		if err == nil || !strings.Contains(err.Error(), "relative to the module root") {
			t.Errorf("%s: err = %v", rel, err)
		}
	}
}
