package example_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTheEmbeddedBuildIsNewerThanTheFrontendSource guards every other test in
// this module. They serve web/build, which is embedded when the test binary
// compiles and written only by `just build`, so an edit to a component, a
// generated stub or the adapter after the last build leaves them testing
// yesterday's frontend — and passing.
//
// The build's own clock is the manifest the adapter writes last. Everything
// that decides what the build contains is the frontend's source: the vite
// root minus what the build writes or installs, and the adapter package the
// build runs. Go files beside them are the server's, not the build's.
func TestTheEmbeddedBuildIsNewerThanTheFrontendSource(t *testing.T) {
	t.Parallel()
	const manifest = "web/build/skgo.manifest.json"
	built, err := os.Stat(filepath.FromSlash(manifest))
	if err != nil {
		t.Fatalf("there is no frontend build (%v). Run `just build`.", err)
	}

	var newest string
	var newestAt time.Time
	visit := func(root string, skip map[string]bool) {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skip[filepath.ToSlash(path)] {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || strings.HasSuffix(path, ".go") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(newestAt) {
				newest, newestAt = path, info.ModTime()
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	visit("web", map[string]bool{"web/build": true, "web/node_modules": true, "web/.svelte-kit": true})
	visit(filepath.Join("..", "internal", "adapter"), map[string]bool{"../internal/adapter/node_modules": true})
	if newest == "" {
		t.Fatal("found no frontend source to compare the build with")
	}

	if newestAt.After(built.ModTime()) {
		t.Fatalf("%s changed at %s, after the embedded build was written at %s: every test here would check the old frontend. Run `just build`.",
			newest, newestAt.Format(time.RFC3339Nano), built.ModTime().Format(time.RFC3339Nano))
	}
}
