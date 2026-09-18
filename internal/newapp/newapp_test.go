package newapp

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHighestVersionIncludesPrereleasesButNotAnotherMajor(t *testing.T) {
	registry := registryServer(t, `{"versions":{"0.17.0":{},"1.0.0-next.7":{},"1.0.0":{},"1.1.0-beta.2":{},"2.0.0-alpha.1":{}}}`)
	got, err := highestVersion(registry.Client(), registry.URL, "sv", func(version string) bool {
		return strings.HasPrefix(version, "1.")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.1.0-beta.2" {
		t.Fatalf("highest sv 1.x = %q, want 1.1.0-beta.2", got)
	}
}

func TestCreateDelegatesFrontendAndWritesOnlyGoOwnedSetup(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.6":{},"1.0.0-next.7":{},"2.0.0":{}}}`)
	dir := filepath.Join(t.TempDir(), "hello-go")
	var commands []command
	runner := func(c command) error {
		commands = append(commands, c)
		if c.Name == "vp-test" {
			writeFrontendFixture(t, c.Dir, true)
		}
		return nil
	}

	result, err := Create(Options{
		Dir:         dir,
		Module:      "example.com/hello-go",
		Starter:     "examples",
		SkgoVersion: "v0.4.1",
		SkgoReplace: t.TempDir(),
		AdapterSpec: "file:/candidate/adapter",
		RegistryURL: registry.URL,
		Client:      registry.Client(),
		VP:          "vp-test",
		run:         runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Starter != "examples" || !strings.Contains(result.Instructions(), "just dev") {
		t.Fatalf("result = %+v; instructions = %q", result, result.Instructions())
	}
	if len(commands) != 4 {
		t.Fatalf("commands = %#v; want VitePlus, Playwright, go mod tidy, go generate", commands)
	}
	if got := commands[0].Args[1]; got != "svelte@1.0.0-next.7" {
		t.Fatalf("VitePlus template = %q", got)
	}
	joined := strings.Join(commands[0].Args, " ")
	for _, want := range []string{"--template minimal", "vitest=usages:unit,component", "storybook", "starter:examples", "adapter:file%3A%2Fcandidate%2Fadapter"} {
		if !strings.Contains(joined, want) {
			t.Errorf("VitePlus args do not contain %q:\n%s", want, joined)
		}
	}
	for _, name := range []string{"go.mod", "server.go", "cmd/main.go", "internal/skgo/config.go", "web/dist.go", "web/src/routes/example.remote.go"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("generated %s: %v", name, err)
		}
	}
	page, err := os.ReadFile(filepath.Join(dir, "web", "src", "routes", "+page.svelte"))
	if err != nil {
		t.Fatal(err)
	}
	if string(page) != "owned by the sv add-on\n" {
		t.Fatalf("Go rewrote the frontend page: %q", page)
	}
}

func TestCreateRefusesSwallowedStorybookFailure(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	dir := filepath.Join(t.TempDir(), "incomplete")
	runner := func(c command) error {
		if c.Name == "vp-test" {
			writeFrontendFixture(t, c.Dir, false)
		}
		return nil
	}
	_, err := Create(Options{
		Dir: dir, Starter: "minimal", SkgoVersion: "v0.4.1", AdapterSpec: "file:/candidate/adapter",
		RegistryURL: registry.URL, Client: registry.Client(), VP: "vp-test", run: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "Storybook") {
		t.Fatalf("Create error = %v; want clear Storybook failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Go setup ran after incomplete frontend: %v", statErr)
	}
}

func TestCreateReportsVitePlusCancellation(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	dir := filepath.Join(t.TempDir(), "cancelled")
	_, err := Create(Options{
		Dir: dir, SkgoVersion: "v0.4.1", AdapterSpec: "file:/candidate/adapter",
		RegistryURL: registry.URL, Client: registry.Client(), VP: "vp-test",
		run: func(command) error { return errors.New("cancelled") },
	})
	if err == nil || !strings.Contains(err.Error(), "VitePlus project creation failed") {
		t.Fatalf("Create error = %v", err)
	}
}

func registryServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func writeFrontendFixture(t *testing.T, root string, complete bool) {
	t.Helper()
	files := map[string]string{
		"web/package.json":            `{"scripts":{"dev":"vp dev","build":"vp build","test:unit":"vp test","storybook":"storybook dev"},"devDependencies":{"@skgo/sveltekit-adapter":"file:/candidate/adapter"}}`,
		"web/src/routes/+page.svelte": "owned by the sv add-on\n",
		"web/src/lib/greet.spec.ts":   "test",
	}
	if complete {
		files["web/.storybook/main.ts"] = "export default {}"
		files["web/src/stories/Button.stories.svelte"] = "story"
		for _, executable := range []string{"vp", "vitest", "storybook"} {
			files["web/node_modules/.bin/"+executable] = ""
		}
	}
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
