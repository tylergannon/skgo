package example_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestChangingAGoTypeBreaksTheComponentThatUsesIt is the end-to-end types
// claim, checked rather than asserted.
//
// It renames one field on the wire — the json tag of businesslogic.Todo.Text —
// regenerates, and requires the SvelteKit type check to fail naming the
// component that reads the old field. Without generated TypeScript coming from
// the Go type, the change would sail through to the browser and show up as an
// empty todo.
//
// The Go source is restored and regenerated whatever happens.
func TestChangingAGoTypeBreaksTheComponentThatUsesIt(t *testing.T) {
	requireFrontendToolchain(t)

	source := filepath.Join("businesslogic", "store.go")
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("reading %s: %v", source, err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(source, original, 0o644); err != nil {
			t.Fatalf("restoring %s: %v", source, err)
		}
		if out, err := generate(); err != nil {
			t.Fatalf("regenerating after the test: %v\n%s", err, out)
		}
	})

	// The check has to be able to fail, so start from a passing state.
	if out, err := svelteCheck(); err != nil {
		t.Fatalf("the app does not type-check before the change: %v\n%s", err, out)
	}

	const tag = "`json:\"text\"`"
	if !bytes.Contains(original, []byte(tag)) {
		t.Fatalf("%s no longer contains %s; update this test", source, tag)
	}
	changed := bytes.Replace(original, []byte(tag), []byte("`json:\"label\"`"), 1)
	if err := os.WriteFile(source, changed, 0o644); err != nil {
		t.Fatalf("writing %s: %v", source, err)
	}

	if out, err := generate(); err != nil {
		t.Fatalf("regenerating after the change: %v\n%s", err, out)
	}

	types, err := os.ReadFile(filepath.Join("web", "src", "lib", "skgo", "businesslogic", "types.ts"))
	if err != nil {
		t.Fatalf("reading the generated types: %v", err)
	}
	if !bytes.Contains(types, []byte(`"label": string`)) {
		t.Fatalf("the generated TypeScript did not follow the Go type:\n%s", types)
	}

	out, err := svelteCheck()
	if err == nil {
		t.Fatal("the app still type-checks after the Go type changed; the components are not typed by Go")
	}
	if !strings.Contains(out, "TodoList.svelte") || !strings.Contains(out, "'text'") {
		t.Fatalf("the type check failed, but not on the component that reads the renamed field:\n%s", out)
	}
	t.Logf("caught before the browser:\n%s", out)
}

func generate() (string, error) {
	cmd := exec.Command("go", "generate", "./...")
	cmd.Dir = "generated"
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// svelteCheck runs the app's own type check, which is what `svelte-check`
// exists for: TypeScript alone cannot read a `.svelte` file.
func svelteCheck() (string, error) {
	cmd := exec.Command("mise", "x", "--",
		"node", filepath.Join("node_modules", "svelte-check", "bin", "svelte-check"),
		"--tsconfig", "./tsconfig.json", "--output", "human")
	cmd.Dir = "web"
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// requireFrontendToolchain skips when the JavaScript side is not installed,
// which is the normal state of a fresh checkout.
func requireFrontendToolchain(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		filepath.Join("web", "node_modules", "svelte-check"),
		filepath.Join("web", ".svelte-kit", "types"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s is missing; run `vp install` and `vp build` in example/web first", path)
		}
	}
	if _, err := exec.LookPath("mise"); err != nil {
		t.Skip("mise is not installed")
	}
}
