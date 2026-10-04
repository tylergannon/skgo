package gen

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoPagePrerenderRedirectBuildsKitsNativeArtifact exercises the complete
// build bridge: a Go page load redirects while Kit crawls /old, and the adapter
// must retain both the recorded path and Kit's HTML output.
func TestGoPagePrerenderRedirectBuildsKitsNativeArtifact(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, filepath.Join(t.TempDir(), "example"))
	if err != nil {
		t.Fatal(err)
	}
	ui := filepath.Join(app, "ui")
	if err := os.Rename(filepath.Join(app, "web"), ui); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(app, "internal", "skgo", "config.go")
	if err := replaceOnce(config, "--web ../../web", "--web ../../ui"); err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(app, "go.mod")
	if err := replaceOnce(goMod, "ignore ./web/node_modules", "ignore ./ui/node_modules"); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "cmd", "main.go"), "github.com/tylergannon/skgo/example/web", "github.com/tylergannon/skgo/example/ui"); err != nil {
		t.Fatal(err)
	}
	linkExampleFrontendDependencies(t, root, app)

	write := func(name, content string) {
		t.Helper()
		file := filepath.Join(ui, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/routes/old/page.server.go", `package old

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Data struct{}

func pageLoad(context.Context) (Data, error) {
	return Data{}, &skgo.Redirect{Status: 307, Location: "/target?from=atlas"}
}

var _ = skgo.Load(pageLoad)
`)
	write("src/routes/old/+page.ts", "export const prerender = true;\n")
	write("src/routes/old/+page.svelte", "<h1>Old route</h1>\n")
	write("src/routes/target/+page.svelte", "<h1>Target route</h1>\n")

	if output, err := runGoGenerate(app); err != nil {
		t.Fatalf("go generate ./...: %v\n%s", err, output)
	}
	cmd := exec.Command(filepath.Join(ui, "node_modules", ".bin", "vp"), "build")
	cmd.Dir = ui
	cmd.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	t.Logf("vp build exit: %d\nstdout:\n%s\nstderr:\n%s", exitCode(err), stdout.String(), stderr.String())
	if err != nil {
		t.Fatalf("vp build: %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(ui, "build", "skgo.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Prerendered []string `json:"prerendered"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if !contains(manifest.Prerendered, "/old") {
		t.Fatalf("Kit's recorded prerender paths omit /old: %v", manifest.Prerendered)
	}
	artifact, err := os.ReadFile(filepath.Join(ui, "build", "prerendered", "old.html"))
	if err != nil {
		t.Fatalf("Kit did not write the native redirect artifact: %v", err)
	}
	for _, want := range []string{`location.href="/target?from=atlas"`, `http-equiv="refresh"`, `url=/target?from=atlas`} {
		if !strings.Contains(string(artifact), want) {
			t.Errorf("native redirect artifact lacks %q: %s", want, artifact)
		}
	}

	build := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "skgo-example"), "./cmd")
	build.Dir = app
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd: %v\n%s", err, output)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	return -1
}

func replaceOnce(path, old, replacement string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := strings.Replace(string(content), old, replacement, 1)
	if updated == string(content) {
		return os.ErrNotExist
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
