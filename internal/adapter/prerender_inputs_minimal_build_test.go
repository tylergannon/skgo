package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// minimalInputsTemp outlives individual tests: consumers only read the shared build.
var minimalInputsTemp string
var sharedInputsTemp string

func TestMain(m *testing.M) {
	var err error
	sharedInputsTemp, err = os.MkdirTemp("", "skgo-adapter-shared-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(sharedInputsTemp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	if minimalInputsTemp != "" {
		if err := os.RemoveAll(minimalInputsTemp); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

func stageMinimalInputsBootstrap(fixture string) error {
	root, err := filepath.Abs("../../example")
	if err != nil {
		return err
	}
	for _, file := range []string{"go.mod", "go.sum", "web/package.json", "web/vite.config.ts", "web/tsconfig.json", "web/src/app.html"} {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return err
		}
		if file == "go.mod" {
			updated := strings.Replace(string(contents), "replace github.com/tylergannon/skgo => ../", "replace github.com/tylergannon/skgo => "+filepath.Clean(filepath.Join(root, "..")), 1)
			if updated == string(contents) {
				return fmt.Errorf("could not point the isolated minimal app module at this worktree")
			}
			contents = []byte(updated)
		}
		if file == "web/package.json" {
			contents = []byte(strings.Replace(string(contents), "link:../../internal/adapter", "link:"+filepath.ToSlash(filepath.Join(root, "..", "internal", "adapter")), 1))
		}
		if file == "web/vite.config.ts" {
			// These fixtures assert native artifacts and service lifecycle, not
			// compressed variants. Keep their real build without extra encoders.
			updated := strings.Replace(string(contents), "adapter: skgo(),", "adapter: skgo({ precompress: false }),", 1)
			if updated == string(contents) {
				return fmt.Errorf("fixture adapter compression option did not apply")
			}
			contents = []byte(updated)
		}
		target := filepath.Join(fixture, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			return err
		}
	}
	dependencies := filepath.Join(root, "web", "node_modules")
	// Kit's no-client-build path populates output/client from static assets.
	// Include a real asset so its crawler has that directory even without JS.
	static := filepath.Join(fixture, "web", "static")
	if err := os.MkdirAll(static, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(static, "fixture.txt"), []byte("fixture\n"), 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dependencies, ".bin", "vp")); err != nil {
		return fmt.Errorf("pinned frontend dependencies are missing: %w", err)
	}
	return linkFixtureDependencyTree(dependencies, filepath.Join(fixture, "web", "node_modules"), filepath.Join(root, "..", "internal", "adapter"))
}

// Mutable adjacent contracts still get their own minimal application.
func prepareMinimalInputsApp(t *testing.T) string {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "example")
	if err := stageMinimalInputsBootstrap(fixture); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(fixture, "internal", "skgo", "config.go"), "package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app\n")
	writeFixtureFile(t, filepath.Join(fixture, "internal", "app", "locals.go"), "package app\ntype Locals struct{}\n")
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.svelte"), `<h1>Minimal declared Inputs fixture</h1>`)
	return fixture
}

var generatedMinimalInputsApp = sync.OnceValues(func() (string, error) {
	var err error
	minimalInputsTemp, err = os.MkdirTemp("", "skgo-adapter-minimal-")
	if err != nil {
		return "", err
	}
	fixture := filepath.Join(minimalInputsTemp, "example")
	if err := stageMinimalInputsBootstrap(fixture); err != nil {
		return "", err
	}
	if err := os.CopyFS(fixture, os.DirFS("testdata/minimal-inputs")); err != nil {
		return "", err
	}
	// The same successful build demonstrates literal zero predicate traffic
	// from a typed nil Handle, as well as the native remote artifacts below.
	config := filepath.Join(fixture, "internal", "skgo", "config.go")
	contents, err := os.ReadFile(config)
	if err != nil {
		return "", err
	}
	contents = []byte(strings.Replace(string(contents), "--locals-package github.com/tylergannon/skgo/example/internal/app", "--locals-package github.com/tylergannon/skgo/example/internal/app --hook-package github.com/tylergannon/skgo/example/internal/serverhooks", 1))
	if err := os.WriteFile(config, contents, 0o644); err != nil {
		return "", err
	}
	hooks := filepath.Join(fixture, "internal", "serverhooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(hooks, "handle.go"), []byte("package serverhooks\nimport \"github.com/tylergannon/skgo/example/internal/skgo/params\"\nvar Handle params.Middleware\n"), 0o644); err != nil {
		return "", err
	}
	receipt := filepath.Join(fixture, "inputs-receipt")
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	// Setting the receipt for generation makes premature producer execution observable.
	generate.Env = append(fixtureBuildEnv(), "GOWORK=off", "SKGO_INPUTS_RECEIPT="+receipt)
	if output, err := generate.CombinedOutput(); err != nil {
		return "", fmt.Errorf("minimal app go generate: %w\n%s", err, output)
	}
	if err := checkMinimalInputsLoads(fixture); err != nil {
		return "", err
	}
	if _, err := os.Stat(receipt); !os.IsNotExist(err) {
		return "", fmt.Errorf("producer ran during generation: %v", err)
	}
	return fixture, nil
})

var builtMinimalInputsApp = sync.OnceValues(func() (string, error) {
	fixture, err := generatedMinimalInputsApp()
	if err != nil {
		return "", err
	}
	receipt := filepath.Join(fixture, "inputs-receipt")
	build := exec.Command("node", "-e", nilCallbacksBuildProgram())
	build.Dir = filepath.Join(fixture, "web")
	build.Env = append(fixtureBuildEnv(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "SKGO_INPUTS_RECEIPT="+receipt)
	output, err := build.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("minimal native build: %w\n%s", err, output)
	}
	if !strings.Contains(string(output), "NIL_CALLBACK_TRAFFIC:0 OWNER_CHANNEL_CLOSED") {
		return "", fmt.Errorf("native nil callback assertions did not finish: %s", output)
	}
	return fixture, nil
})

func requireMinimalInputsBuild(t *testing.T) string {
	t.Helper()
	fixture, err := builtMinimalInputsApp()
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func writeFixtureFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func generateMinimalInputsApp(t *testing.T, fixture string) {
	t.Helper()
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(fixtureBuildEnv(), "GOWORK=off")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("minimal app go generate: %v\n%s", err, output)
	}
	if err := checkMinimalInputsLoads(fixture); err != nil {
		t.Fatal(err)
	}
}

func checkMinimalInputsLoads(fixture string) error {
	manifest, err := os.ReadFile(filepath.Join(fixture, "web", "skgo.remotes.json"))
	if err != nil {
		return err
	}
	var generated struct {
		Loads []string `json:"loads"`
	}
	if err := json.Unmarshal(manifest, &generated); err != nil {
		return err
	}
	if len(generated.Loads) != 0 {
		return fmt.Errorf("minimal fixture unexpectedly contains Go load handlers: %v", generated.Loads)
	}
	return nil
}

func buildMinimalInputsApp(t *testing.T, fixture string, env ...string) (string, error) {
	t.Helper()
	return runMinimalInputsBuild(fixture, env...)
}

func runMinimalInputsBuild(fixture string, env ...string) (string, error) {
	build := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	build.Dir = filepath.Join(fixture, "web")
	build.Env = append(fixtureBuildEnv(), append([]string{"GOWORK=off", "ORIGIN=http://127.0.0.1:8080"}, env...)...)
	output, err := build.CombinedOutput()
	return string(output), err
}

func TestMinimalNoGoLoadsInputsBuildProducesNoArgumentArtifact(t *testing.T) {
	t.Parallel()
	fixture := requireMinimalInputsBuild(t)
	app := filepath.Join(fixture, "web")
	receipt := filepath.Join(fixture, "inputs-receipt")
	artifact := filepath.Join(app, "build", "prerendered", "_app", "remote", "3215r6", "empty")
	contents, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("minimal no-argument artifact: %v", err)
	}
	const want = `{"type":"result","data":"[{\"_\":1,\"p\":2},\"build:empty\",{\"3215r6/empty/\":3},{\"v\":1}]"}`
	if string(contents) != want {
		t.Fatalf("minimal no-argument native artifact = %s; want independent literal %s", contents, want)
	}
	if got, err := os.ReadFile(receipt); err != nil || string(got) != "inputs:empty\nbody:empty\n" {
		t.Fatalf("minimal producer/body receipt = %q, %v", got, err)
	}
}

func TestSameExportAcrossPackagesInputsBuildProducesBothArtifacts(t *testing.T) {
	t.Parallel()
	fixture := requireMinimalInputsBuild(t)
	app := filepath.Join(fixture, "web")
	artifacts := []struct{ path, want string }{
		{"1vyw5d0/item/WyJhdGxhcyJd", `{"type":"result","data":"[{\"_\":1,\"p\":2},\"alpha:atlas\",{\"1vyw5d0/item/WyJhdGxhcyJd\":3},{\"v\":1}]"}`},
		{"1h38tvo/item/WzVd", `{"type":"result","data":"[{\"_\":1,\"p\":2},25,{\"1h38tvo/item/WzVd\":3},{\"v\":1}]"}`},
	}
	for _, artifact := range artifacts {
		contents, err := os.ReadFile(filepath.Join(app, "build", "prerendered", "_app", "remote", filepath.FromSlash(artifact.path)))
		if err != nil || string(contents) != artifact.want {
			t.Errorf("same-export native artifact %s = %q, %v; want independent literal %s", artifact.path, contents, err, artifact.want)
		}
	}
}
