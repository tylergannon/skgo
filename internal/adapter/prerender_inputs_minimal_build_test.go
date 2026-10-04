package adapter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func prepareMinimalInputsApp(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "example")
	copyFixtureTree(t, root, fixture)
	dependencies := filepath.Join(root, "web", "node_modules")
	if _, err := os.Stat(filepath.Join(dependencies, ".bin", "vp")); err != nil {
		t.Fatalf("pinned frontend dependencies are missing: %v", err)
	}
	linkFixtureDependencies(t, dependencies, filepath.Join(fixture, "web", "node_modules"), filepath.Join(root, "..", "internal", "adapter"))

	modulePath := filepath.Join(fixture, "go.mod")
	module, err := os.ReadFile(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(module), "replace github.com/tylergannon/skgo => ../", "replace github.com/tylergannon/skgo => "+filepath.Clean(filepath.Join(root, "..")), 1)
	if updated == string(module) {
		t.Fatal("could not point the isolated minimal app module at this worktree")
	}
	if err := os.WriteFile(modulePath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, entry := range []string{"web", "go.mod", "go.sum", "mise.toml"} {
		if _, err := os.Stat(filepath.Join(fixture, entry)); err != nil && entry != "mise.toml" {
			t.Fatalf("minimal app fixture is missing %s: %v", entry, err)
		}
	}
	entries, err := os.ReadDir(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "web", "go.mod", "go.sum", "mise.toml":
		default:
			if err := os.RemoveAll(filepath.Join(fixture, entry.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.RemoveAll(filepath.Join(fixture, "web", "src")); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(fixture, "internal", "skgo", "config.go"), "package skgo\n//go:generate go tool skgo generate --web ../../web\n")
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "app.html"), `<!doctype html><html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>`)
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.svelte"), `<h1>Minimal declared Inputs fixture</h1>`)
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
	generate.Env = append(os.Environ(), "GOWORK=off")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("minimal app go generate: %v\n%s", err, output)
	}
	manifest, err := os.ReadFile(filepath.Join(fixture, "web", "skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated struct {
		Loads []string `json:"loads"`
	}
	if err := json.Unmarshal(manifest, &generated); err != nil {
		t.Fatal(err)
	}
	if len(generated.Loads) != 0 {
		t.Fatalf("minimal fixture unexpectedly contains Go load handlers: %v", generated.Loads)
	}
}

func buildMinimalInputsApp(t *testing.T, fixture string, env ...string) (string, error) {
	t.Helper()
	build := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	build.Dir = filepath.Join(fixture, "web")
	build.Env = append(os.Environ(), append([]string{"GOWORK=off", "ORIGIN=http://127.0.0.1:8080"}, env...)...)
	output, err := build.CombinedOutput()
	return string(output), err
}

func TestMinimalNoGoLoadsInputsBuildProducesNoArgumentArtifact(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	app := filepath.Join(fixture, "web")
	writeFixtureFile(t, filepath.Join(app, "src", "lib", "fixture.remote.go"), `package lib
import (
  "context"
  "fmt"
  "os"
  "github.com/tylergannon/polytype/devalue"
  "github.com/tylergannon/skgo"
)
func note(line string) {
  path := os.Getenv("SKGO_INPUTS_RECEIPT")
  if path == "" { return }
  file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
  if err != nil { panic(err) }
  defer file.Close()
  if _, err := fmt.Fprintln(file, line); err != nil { panic(err) }
}
func emptyInputs() ([]devalue.UndefinedValue, error) { note("inputs:empty"); return []devalue.UndefinedValue{}, nil }
func empty(context.Context) (string, error) { note("body:empty"); return "build:empty", nil }
var _ = skgo.Prerender(empty, skgo.PrerenderOptions{Inputs: emptyInputs})
`)
	writeFixtureFile(t, filepath.Join(app, "src", "routes", "+layout.ts"), `import '../lib/fixture.remote';
export const prerender = true;
`)
	generateMinimalInputsApp(t, fixture)
	receipt := filepath.Join(fixture, "inputs-receipt")
	if _, err := os.Stat(receipt); !os.IsNotExist(err) {
		t.Fatalf("producer ran during generation: %v", err)
	}
	output, err := buildMinimalInputsApp(t, fixture, "SKGO_INPUTS_RECEIPT="+receipt)
	if err != nil {
		t.Fatalf("minimal no-GoLoads native build: %v\n%s", err, output)
	}
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
	fixture := prepareMinimalInputsApp(t)
	app := filepath.Join(fixture, "web")
	writeFixtureFile(t, filepath.Join(app, "src", "alpha", "alpha.remote.go"), `package alpha
import (
  "context"
  "github.com/tylergannon/skgo"
)
func item(_ context.Context, value string) (string, error) { return "alpha:" + value, nil }
func names() ([]string, error) { return []string{"atlas"}, nil }
var _ = skgo.Prerender(item, skgo.PrerenderOptions{Inputs: names})
`)
	writeFixtureFile(t, filepath.Join(app, "src", "beta", "beta.remote.go"), `package beta
import (
  "context"
  "github.com/tylergannon/skgo"
)
func item(_ context.Context, value int) (int, error) { return value + 20, nil }
func names() ([]int, error) { return []int{5}, nil }
var _ = skgo.Prerender(item, skgo.PrerenderOptions{Inputs: names})
`)
	writeFixtureFile(t, filepath.Join(app, "src", "routes", "+layout.ts"), `import '../alpha/alpha.remote';
import '../beta/beta.remote';
export const prerender = true;
`)
	generateMinimalInputsApp(t, fixture)
	output, err := buildMinimalInputsApp(t, fixture)
	if err != nil {
		t.Fatalf("same-export packages native build: %v\n%s", err, output)
	}
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
