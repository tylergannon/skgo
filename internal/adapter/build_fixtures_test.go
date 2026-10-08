package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Native builds keep separate Vite roots, processes and private compile output.
// Their Go service can share an immutable generated module: mode selection is
// through each child's environment, and go build still runs at the real boundary.
func cloneGeneratedInputsWeb(t *testing.T, source string) string {
	t.Helper()
	fixture := prepareMinimalInputsApp(t)
	web := filepath.Join(fixture, "web")
	if err := os.RemoveAll(filepath.Join(web, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(web, "src"), os.DirFS(filepath.Join(source, "web", "src"))); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vite.config.ts", "skgo.remotes.json"} {
		contents, err := os.ReadFile(filepath.Join(source, "web", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "skgo.remotes.json" {
			var manifest map[string]json.RawMessage
			if err := json.Unmarshal(contents, &manifest); err != nil {
				t.Fatal(err)
			}
			var service struct{ Root, Package string }
			if err := json.Unmarshal(manifest["prerender"], &service); err != nil || service.Package == "" {
				t.Fatalf("missing generated service: %v", err)
			}
			serviceRoot := filepath.Join(source, "web", service.Root)
			relative, err := filepath.Rel(web, serviceRoot)
			if err != nil {
				t.Fatal(err)
			}
			manifest["prerender"], err = json.Marshal(struct {
				Root    string `json:"root"`
				Package string `json:"package"`
			}{filepath.ToSlash(relative), service.Package})
			if err != nil {
				t.Fatal(err)
			}
			contents, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
		}
		writeFixtureFile(t, filepath.Join(web, name), string(contents))
	}
	return fixture
}

// Keep production policy independent of the controlled deadlines used by the
// real owner/worker timeout builds. No application configuration is added.
func TestPrerenderProductionTimeoutPolicy(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(source), "const CALLBACK_TIMEOUT = 30_000;") != 1 {
		t.Fatal("production callback deadline must remain 30 seconds")
	}
}

func installInputsTimeoutAdapter(t *testing.T, fixture string, mode string) {
	t.Helper()
	root := filepath.Join(fixture, "adapter")
	for _, name := range packageFiles {
		contents, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if name == "skgo-adapter/prerender.js" {
			const policy = "const CALLBACK_TIMEOUT = 30_000;"
			if strings.Count(string(contents), policy) != 1 {
				t.Fatal("callback timeout policy injection did not apply")
			}
			deadline := "const CALLBACK_TIMEOUT = 500;"
			if mode == "worker-timeout" {
				deadline = "const CALLBACK_TIMEOUT = isMainThread ? 10_000 : 500;"
			} else if mode == "timeout" {
				deadline = "const CALLBACK_TIMEOUT = isMainThread ? 500 : 10_000;"
			}
			contents = []byte(strings.Replace(string(contents), policy, deadline, 1))
		}
		writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(name)), string(contents))
	}
	link := filepath.Join(fixture, "web", "node_modules", "@skgo", "sveltekit-adapter")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
}

func prepareDeclaredInputsApp(t *testing.T) string {
	t.Helper()
	fixture := prepareMinimalInputsApp(t)
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"web/src/routes/go.mod", "businesslogic/money.go", "web/src/hooks.go", "web/src/hooks.ts", "web/dist.go",
		"web/src/routes/about/about.remote.go", "web/src/routes/about/+page.ts",
		"web/src/routes/about/+page.svelte", "web/src/routes/about/page.server.go",
		"web/src/routes/prerender/[slug]/+page.ts", "web/src/routes/prerender/[slug]/+page.svelte",
		"web/src/routes/prerender/[slug]/page.server.go",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(fixture, filepath.FromSlash(name)), string(data))
	}
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "layout.server.go"), `package site
import "github.com/tylergannon/skgo"
type RootLayoutData struct { Deployment string `+"`json:\"deployment\"`"+` }
func layoutLoad(LayoutRequestEvent) (RootLayoutData,error) { return RootLayoutData{Deployment:"skgo example"},nil }
var _ = skgo.Load(layoutLoad)
`)
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "lib", "fixture.remote.go"), `package lib
import (
  "context"
  "fmt"
  "os"
  "github.com/tylergannon/skgo"
)
func note(value string) {
  path := os.Getenv("SKGO_IDENTIFIER_RECEIPT")
  if path == "" { return }
  file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
  if err != nil { panic(err) }
  defer file.Close()
  if _, err := fmt.Fprintln(file, value); err != nil { panic(err) }
}
func skgoRemoteInputs(_ context.Context, value string) (string, error) {
  note("body")
  return "body:" + value, nil
}
func fixtureInputs() ([]string, error) {
  note("producer")
  return []string{"atlas"}, nil
}
var _ = skgo.Prerender(skgoRemoteInputs, skgo.PrerenderOptions{Inputs: fixtureInputs})
`)
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.ts"), "export const prerender=true;\n")
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.svelte"), `<script>
 import { skgoRemoteInputs } from '../lib/fixture.remote';
 const result=skgoRemoteInputs('atlas');
</script><h1>{#await result then value}{value}{/await}</h1>`)
	// The handler test compiles this package alongside the embedded app output.
	writeFixtureFile(t, filepath.Join(fixture, "cmd", "doc.go"), "package main\n")
	return fixture
}

// Fixtures compile Go and bundle through Rust concurrently. Give each child
// a bounded budget instead of multiplying whole-machine worker pools.
func fixtureBuildEnv() []string {
	env := replaceEnv(os.Environ(), "GOMAXPROCS", "1")
	// Disposable executables exercise compiled Go code, not debugger metadata.
	// Keep the real compiler/linker boundary while avoiding repeated DWARF output.
	env = replaceEnv(env, "GOFLAGS", os.Getenv("GOFLAGS")+" -ldflags=-w")
	env = replaceEnv(env, "RAYON_NUM_THREADS", "1")
	env = replaceEnv(env, "ROLLDOWN_WORKER_THREADS", "1")
	env = replaceEnv(env, "ROLLDOWN_MAX_BLOCKING_THREADS", "1")
	return env
}
