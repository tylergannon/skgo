package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type productionFixture struct {
	app   string
	tests string
}

var preparedProductionFixture = sync.OnceValues(func() (productionFixture, error) {
	root, err := repoRoot()
	if err != nil {
		return productionFixture{}, err
	}
	app := filepath.Join(packageTemp, "prerender-production")
	if err := stagePrerenderFixture(root, app, "prerender-production"); err != nil {
		return productionFixture{}, err
	}
	return productionFixture{app: app}, nil
})

// Build output is read-only after this once completes. Each consumer executes
// the compiled ordinary Go tests with fresh process and handler state.
var builtProductionFixture = sync.OnceValues(func() (productionFixture, error) {
	fixture, err := preparedProductionFixture()
	if err != nil {
		return productionFixture{}, err
	}
	if output, err := runGoGenerate(fixture.app); err != nil {
		return productionFixture{}, fmt.Errorf("go generate ./...: %w\n%s", err, output)
	}
	ui := filepath.Join(fixture.app, "ui")
	build := exec.Command(filepath.Join(ui, "node_modules", ".bin", "vp"), "build")
	build.Dir = ui
	build.Env = append(os.Environ(), "ORIGIN=http://127.0.0.1:8080")
	if output, err := build.CombinedOutput(); err != nil {
		return productionFixture{}, fmt.Errorf("vp build under ui: %w\n%s", err, output)
	}
	command := exec.Command("go", "build", "-o", filepath.Join(fixture.app, "server"), "./cmd")
	command.Dir = fixture.app
	if output, err := command.CombinedOutput(); err != nil {
		return productionFixture{}, fmt.Errorf("compile production binary importing ui: %w\n%s", err, output)
	}
	fixture.tests = filepath.Join(fixture.app, "production.test")
	compile := exec.Command("go", "test", "-c", "-o", fixture.tests, ".")
	compile.Dir = fixture.app
	if output, err := compile.CombinedOutput(); err != nil {
		return productionFixture{}, fmt.Errorf("compile production handler contracts: %w\n%s", err, output)
	}
	return fixture, nil
})

func requireProductionFixture(t *testing.T) productionFixture {
	t.Helper()
	fixture, err := preparedProductionFixture()
	if err != nil {
		t.Fatal(err)
	}
	// Linking needs testing.T for diagnostics; it must also run only once.
	linkProductionDependencies.Do(func() {
		root, err := repoRoot()
		if err != nil {
			t.Fatal(err)
		}
		linkExampleFrontendDependencies(t, root, fixture.app)
	})
	fixture, err = builtProductionFixture()
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

var linkProductionDependencies sync.Once

func (f productionFixture) run(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command(f.tests, "-test.run=^"+name+"$", "-test.v")
	cmd.Dir = f.app
	output, err := cmd.CombinedOutput()
	t.Logf("compiled production handler contract:\n%s", output)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !strings.Contains(string(output), "=== RUN   "+name+"\n") ||
		!strings.Contains(string(output), "--- PASS: "+name+" (") ||
		strings.Contains(string(output), "--- SKIP:") {
		t.Fatalf("%s did not execute and pass without skips:\n%s", name, output)
	}
}
