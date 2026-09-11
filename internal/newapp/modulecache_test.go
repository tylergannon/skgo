package newapp_test

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/mod/module"
)

// removeFromModuleCache deletes one module version from the module cache
// rooted at cache: the tree the go command extracted it into, and the files it
// downloaded to get there. Every other version belongs to whichever go command
// is using it, which does not expect it to vanish.
//
// The go command writes and reads the version list itself, under its own lock,
// and rebuilds it from the .mod files present the next time it adds a version,
// so it is left alone.
func removeFromModuleCache(cache string, mv module.Version) error {
	path, err := module.EscapePath(mv.Path)
	if err != nil {
		return err
	}
	version, err := module.EscapeVersion(mv.Version)
	if err != nil {
		return err
	}

	// The extracted tree is read-only, and a directory has to be writable for
	// anything in it to be removed.
	extracted := filepath.Join(cache, filepath.FromSlash(path)+"@"+version)
	_ = filepath.WalkDir(extracted, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o755)
		}
		return nil
	})
	errs := []error{os.RemoveAll(extracted)}

	downloads := filepath.Join(cache, "cache", "download", filepath.FromSlash(path), "@v")
	for _, ext := range []string{".info", ".mod", ".zip", ".ziphash", ".lock", ".partial"} {
		err := os.Remove(filepath.Join(downloads, version+ext))
		if !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// TestModuleCacheCleanupRemovesOnlyItsOwnVersion is the trip-wire for the one
// way the scaffold tests have broken each other: a cleanup that cleared every
// scaffold version at once, including the one a concurrent run was building
// from. Cleaning one version has to leave every other one, scaffold or not,
// exactly as it was.
func TestModuleCacheCleanupRemovesOnlyItsOwnVersion(t *testing.T) {
	cache := t.TempDir()
	mine := module.Version{Path: "github.com/tylergannon/skgo", Version: "v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa"}

	removed := map[string]string{
		"github.com/tylergannon/skgo@v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa/go.mod":                    "module mine",
		"github.com/tylergannon/skgo@v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa/cmd/skgo/main.go":          "package main // mine",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa.info":    `{"Version":"mine"}`,
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa.mod":     "module mine",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa.zip":     "zip mine",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa.ziphash": "h1:mine",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa.lock":    "",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa.partial": "",
	}
	kept := map[string]string{
		// Another run of the scaffold test, mid-build.
		"github.com/tylergannon/skgo@v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb/go.mod":                    "module theirs",
		"github.com/tylergannon/skgo@v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb/cmd/skgo/main.go":          "package main // theirs",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb.info":    `{"Version":"theirs"}`,
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb.mod":     "module theirs",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb.zip":     "zip theirs",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb.ziphash": "h1:theirs",
		"cache/download/github.com/tylergannon/skgo/@v/v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb.lock":    "",
		// A released skgo some other project depends on.
		"github.com/tylergannon/skgo@v0.3.0/go.mod":                "module released",
		"cache/download/github.com/tylergannon/skgo/@v/v0.3.0.mod": "module released",
		"cache/download/github.com/tylergannon/skgo/@v/v0.3.0.zip": "zip released",
		"cache/download/github.com/tylergannon/skgo/@v/list":       "v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa\nv0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb\nv0.3.0\n",
		// A module that is nothing to do with skgo.
		"golang.org/x/mod@v0.20.0/go.mod":                "module unrelated",
		"cache/download/golang.org/x/mod/@v/v0.20.0.zip": "zip unrelated",
	}

	for rel, body := range removed {
		writeCacheFile(t, cache, rel, body)
	}
	for rel, body := range kept {
		writeCacheFile(t, cache, rel, body)
	}
	// The go command leaves what it extracts read-only.
	for _, tree := range []string{
		"github.com/tylergannon/skgo@v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa",
		"github.com/tylergannon/skgo@v0.0.0-scaffoldtest-bbbbbbbbbbbbbbbb",
		"github.com/tylergannon/skgo@v0.3.0",
		"golang.org/x/mod@v0.20.0",
	} {
		setTreeMode(t, filepath.Join(cache, filepath.FromSlash(tree)), 0o555)
	}
	t.Cleanup(func() { setTreeMode(t, cache, 0o755) })

	if err := removeFromModuleCache(cache, mine); err != nil {
		t.Fatalf("removing %s: %v", mine, err)
	}

	if got := cacheFiles(t, cache); !maps.Equal(got, kept) {
		t.Errorf("after removing %s the cache holds:\n%v\nwant exactly:\n%v", mine, got, kept)
	}
	extracted := filepath.Join(cache, "github.com/tylergannon/skgo@v0.0.0-scaffoldtest-aaaaaaaaaaaaaaaa")
	if _, err := os.Stat(extracted); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the extracted directory for %s is still there: %v", mine, err)
	}
}

func writeCacheFile(t *testing.T, cache, rel, body string) {
	t.Helper()
	at := filepath.Join(cache, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(at, []byte(body), 0o444); err != nil {
		t.Fatal(err)
	}
}

// setTreeMode sets the mode of every directory under root.
func setTreeMode(t *testing.T, root string, mode fs.FileMode) {
	t.Helper()
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	// Deepest first, so that making a parent read-only does not stop its
	// children being reached.
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i], mode); err != nil {
			t.Fatal(err)
		}
	}
}

// cacheFiles reads every file under cache, keyed by its slash-separated path.
func cacheFiles(t *testing.T, cache string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(cache, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
