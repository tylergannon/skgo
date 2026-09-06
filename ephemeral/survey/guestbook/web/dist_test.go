package web

import (
	"io/fs"
	"strings"
	"testing"
)

// TestBuildEmbedsImmutableAssets guards the `all:` prefix on the //go:embed
// directive. Without it the embed still succeeds, but every path under
// build/client/_app/ is missing and the server ships a boot document whose
// scripts all 404.
func TestBuildEmbedsImmutableAssets(t *testing.T) {
	for _, name := range []string{"build/index.html", "build/skgo.manifest.json"} {
		if _, err := fs.Stat(Build, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	var immutable int
	err := fs.WalkDir(Build, "build/client/_app/immutable", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".js") {
			immutable++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking build/client/_app/immutable: %v (is `all:` missing from //go:embed?)", err)
	}
	if immutable == 0 {
		t.Fatal("no .js files under build/client/_app/immutable")
	}
}
