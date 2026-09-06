package example_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestNothingGeneratedWasWrittenByHand runs the app's own generator over a
// copy of this module and requires it to produce, byte for byte, the files
// already in the tree.
//
// It closes the hole the drift checks cannot see. Those compare the generated
// list against the built frontend, so both sides move together when a
// `.remote.ts` is edited in place: replacing a generated body with a
// hand-written implementation leaves every id unchanged and every check green,
// while the app stops being one whose server logic is written in Go. The same
// failure catches generated output that is merely stale.
//
// Generation happens in the sandbox, so a stale tree is reported and never
// silently rewritten under the developer.
func TestNothingGeneratedWasWrittenByHand(t *testing.T) {
	app := sandbox(t)

	if out, err := generate(app); err != nil {
		t.Fatalf("running the generator: %v\n%s", err, out)
	}

	// go.mod is rewritten by the sandbox itself; node_modules is linked, not
	// copied, and .svelte-kit is vite's output rather than skgo's.
	skip := func(rel string) bool {
		return rel == "go.mod" ||
			strings.HasPrefix(rel, filepath.Join("web", "node_modules")+string(filepath.Separator)) ||
			strings.HasPrefix(rel, filepath.Join("web", ".svelte-kit")+string(filepath.Separator))
	}

	var stale []string
	err := filepath.WalkDir(app, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(app, path)
		if err != nil {
			return err
		}
		if skip(rel) {
			return nil
		}

		generated, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		committed, err := os.ReadFile(rel)
		if err != nil {
			if os.IsNotExist(err) {
				stale = append(stale, rel+" (the generator writes it; the tree does not have it)")
				return nil
			}
			return err
		}
		if !bytes.Equal(generated, committed) {
			stale = append(stale, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("comparing the sandbox with the tree: %v", err)
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		t.Fatalf("`go generate ./...` does not reproduce these files:\n  %s\n"+
			"Either they were edited by hand — remote functions are written in Go, not in .remote.ts —\n"+
			"or the generator has not been run since the Go source changed. Run `go generate ./...` in example/generated.",
			strings.Join(stale, "\n  "))
	}
}
