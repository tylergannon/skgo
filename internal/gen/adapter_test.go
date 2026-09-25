package gen

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/adapter"
)

// The unbypassable gate is at startup: `skgo.ReadManifest` refuses a build
// whose adapter is not this program's. It cannot be moved earlier, because a
// developer can swap node_modules after generating.
//
// What can be moved earlier is the ordinary case — an install that resolved a
// different release than go.mod asks for — and these say it is: `go generate`
// takes seconds, and the alternative is finding out after a frontend build and
// a server start.

// installAdapter writes an `@skgo/sveltekit-adapter` into a vite root's node_modules and
// returns the root. The files are the test's own, so what the check reports has
// to come from what the test put there and not from anything skgo carries.
func installAdapter(t *testing.T, version string, contents map[string]string) string {
	t.Helper()
	web := t.TempDir()
	if err := os.MkdirAll(filepath.Join(web, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(web, "node_modules", filepath.FromSlash(adapter.Package))
	contents["package.json"] = `{"name":"` + adapter.Package + `","version":"` + version + `"}`
	for name, body := range contents {
		at := filepath.Join(pkg, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return web
}

func TestGeneratingAgainstAnotherSkgosAdapterIsRefused(t *testing.T) {
	// A version and an adapter no skgo has ever published.
	const theirVersion = "0.1.7"
	web := installAdapter(t, theirVersion, map[string]string{
		"skgo-adapter.js":          "export default function skgo() {}\n",
		"skgo-adapter/env.js":      "export const SSR_TARGET = 'es5';\n",
		"skgo-adapter/polyfill.js": "",
	})

	err := Run(Config{Web: web, Out: filepath.Join(web, "generated")})
	if err == nil {
		t.Fatal("generated against an @skgo/sveltekit-adapter from a different skgo")
	}
	// Both halves, because the developer's next move depends on knowing which
	// one is behind — and the fingerprint, because two installs can call
	// themselves the same version.
	for _, want := range []string{
		theirVersion, adapter.Fingerprint(), adapter.Version(), adapter.Package, "pnpm add",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q:\n%v", want, err)
		}
	}
}

// TestGeneratingAnEmptyAppNeedsNoInstalledAdapter keeps the compatibility
// check from inventing frontend work when there are no Go-facing bindings.
func TestGeneratingAnEmptyAppNeedsNoInstalledAdapter(t *testing.T) {
	web := t.TempDir()
	if err := os.MkdirAll(filepath.Join(web, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(web, "generated")
	if err := Run(Config{Web: web, Out: out}); err != nil {
		t.Fatalf("generating empty bindings: %v", err)
	}
	bindings, err := os.ReadFile(filepath.Join(out, "skgo_bindings_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func Remotes()", "func Loads()", "func Endpoints()"} {
		if !strings.Contains(string(bindings), want) {
			t.Errorf("empty bindings do not contain %q:\n%s", want, bindings)
		}
	}
	list, err := os.ReadFile(filepath.Join(web, "skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(list), "{\n\t\"remotes\": [],\n\t\"loads\": [],\n\t\"actions\": [],\n\t\"endpoints\": {}\n}\n"; got != want {
		t.Errorf("empty remote list = %q, want %q", got, want)
	}
}

// TestGeneratingAgainstThisModulesAdapterIsAllowed is the other side of the
// refusal: an install of the package this directory publishes gets through.
// Without it the check could be refusing everything and both other tests would
// still pass.
func TestGeneratingAgainstThisModulesAdapterIsAllowed(t *testing.T) {
	contents := map[string]string{}
	source := filepath.Join("..", "adapter")
	err := filepath.WalkDir(source, func(at string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(source, at)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "skgo-adapter.js" && !strings.HasPrefix(rel, "skgo-adapter/") {
			return nil
		}
		raw, err := os.ReadFile(at)
		if err != nil {
			return err
		}
		contents[rel] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) < 2 {
		t.Fatalf("copied %d adapter files out of %s; the adapter is more than that", len(contents), source)
	}

	web := installAdapter(t, "0.0.0-dev", contents)
	err = Run(Config{Web: web, Out: filepath.Join(web, "generated")})
	if err != nil {
		t.Fatalf("generating against this module's own adapter was refused: %v", err)
	}
}
