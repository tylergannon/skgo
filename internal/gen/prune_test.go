package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// skgoGenerate runs the generator directly. `go generate ./...` is not used
// after deleting a whole package: it lists the packages first and would still
// visit the bindings' link copy of one this very run removed.
func skgoGenerate(t *testing.T, app string) {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(app)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(requireSkgoBinary(t), "generate", "--web", filepath.Join(resolved, "web"), "--out", ".", "--quiet")
	cmd.Dir = filepath.Join(resolved, "internal", "skgo")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("skgo generate: %v\n%s", err, out)
	}
}

// A route that loses its Go also loses the modules the generator wrote for it.
// Left behind, a `+page.server.ts` stub keeps Kit believing the route has a
// server load, and a `types.ts` keeps declaring a type nothing returns.
func TestGenerateRemovesWhatTheDeletedGoProduced(t *testing.T) {
	app := sandboxExample(t)
	route := filepath.Join(app, "web", "src", "routes", "go-dev")
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(route, rel))
		return err == nil
	}

	// Authored files are never generated output: a route's component and a
	// sibling route's modules must survive.
	sibling := filepath.Join(app, "web", "src", "routes", "stream", "+page.server.ts")
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	skgoGenerate(t, app)
	for _, rel := range []string{"+page.server.ts", "go-dev.remote.ts", "skgo_remotes_gen.go", "skgo_params_gen.go"} {
		if !exists(rel) {
			t.Fatalf("generation did not write %s for the route's Go", rel)
		}
	}

	for _, rel := range []string{"page.server.go", "go-dev.remote.go"} {
		if err := os.Remove(filepath.Join(route, rel)); err != nil {
			t.Fatal(err)
		}
	}
	skgoGenerate(t, app)
	for _, rel := range []string{"+page.server.ts", "go-dev.remote.ts", "types.ts", "skgo_remotes_gen.go", "skgo_params_gen.go"} {
		if exists(rel) {
			t.Errorf("%s outlived the Go that produced it", rel)
		}
	}
	if !exists("+page.svelte") {
		t.Error("the authored component was removed")
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("another route's generated module was removed: %v", err)
	}
}
