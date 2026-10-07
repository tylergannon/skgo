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
	app         string
	tests       string
	buildOutput string
}

var preparedProductionFixture = sync.OnceValues(func() (productionFixture, error) {
	return prepareProductionFixture(false)
})

var preparedOptionsFixture = sync.OnceValues(func() (productionFixture, error) {
	return prepareProductionFixture(true)
})

func prepareProductionFixture(customDefaults bool) (productionFixture, error) {
	root, err := repoRoot()
	if err != nil {
		return productionFixture{}, err
	}
	app := filepath.Join(packageTemp, "prerender-production")
	if customDefaults {
		app += "-options"
	}
	if err := stagePrerenderFixture(root, app, "prerender-production"); err != nil {
		return productionFixture{}, err
	}
	if customDefaults {
		// Defaults need only these fetch routes. Lifecycle, redirects and remote
		// artifact contracts already run against the strict production build.
		routes := filepath.Join(app, "ui/src/routes")
		entries, err := os.ReadDir(routes)
		if err != nil {
			return productionFixture{}, err
		}
		for _, entry := range entries {
			switch entry.Name() {
			case "options-default", "options-rejected", "options-served", "options-api", "options-live-api":
			default:
				if err := os.RemoveAll(filepath.Join(routes, entry.Name())); err != nil {
					return productionFixture{}, err
				}
			}
		}
		if err := copySandboxFile(filepath.Join(root, "internal/gen/testdata/prerender-options/server_test.go"), filepath.Join(app, "server_test.go")); err != nil {
			return productionFixture{}, err
		}
		if err := replaceOnce(filepath.Join(app, "ui", "vite.config.ts"), "adapter: skgo(),", "adapter: skgo({prerenderPackage: './buildservice'}),\n prerender: {handleHttpError: 'ignore', handleUnseenRoutes: 'ignore'},"); err != nil {
			return productionFixture{}, err
		}
	} else {
		// This intentional Kit error belongs only to the permissive options
		// fixture. Ordinary lifecycle proof uses the generated command and
		// Kit's strict defaults, including unseen-route and HTTP errors.
		if err := os.RemoveAll(filepath.Join(app, "ui", "src", "routes", "options-rejected")); err != nil {
			return productionFixture{}, err
		}
	}
	return productionFixture{app: app}, nil
}

// Build output is read-only after this once completes. Each consumer executes
// the compiled ordinary Go tests with fresh process and handler state.
var builtProductionFixture = sync.OnceValues(func() (productionFixture, error) {
	fixture, err := preparedProductionFixture()
	return buildProductionFixture(fixture, err)
})

var builtOptionsFixture = sync.OnceValues(func() (productionFixture, error) {
	fixture, err := preparedOptionsFixture()
	return buildProductionFixture(fixture, err)
})

func buildProductionFixture(fixture productionFixture, err error) (productionFixture, error) {
	if err != nil {
		return productionFixture{}, err
	}
	root, rootErr := repoRoot()
	if rootErr != nil {
		return productionFixture{}, rootErr
	}
	if err := frontendDependencies(root, filepath.Join(fixture.app, "ui/node_modules")); err != nil {
		return productionFixture{}, err
	}
	if output, err := runGoGenerate(fixture.app); err != nil {
		return productionFixture{}, fmt.Errorf("go generate ./...: %w\n%s", err, output)
	}
	ui := filepath.Join(fixture.app, "ui")
	build := exec.Command(filepath.Join(ui, "node_modules", ".bin", "vp"), "build")
	build.Dir = ui
	build.Env = append(os.Environ(), "ORIGIN=http://127.0.0.1:8080")
	output, err := build.CombinedOutput()
	fixture.buildOutput = string(output)
	if err != nil {
		return productionFixture{}, fmt.Errorf("vp build under ui: %w\n%s", err, output)
	}
	// Only the strict fixture starts the ordinary server binary. The options
	// fixture's served-page contract runs in the compiled handler test below.
	if !strings.HasSuffix(fixture.app, "-options") {
		command := exec.Command("go", "build", "-o", filepath.Join(fixture.app, "server"), "./cmd")
		command.Dir = fixture.app
		if output, err := command.CombinedOutput(); err != nil {
			return productionFixture{}, fmt.Errorf("compile production binary importing ui: %w\n%s", err, output)
		}
	}
	fixture.tests = filepath.Join(fixture.app, "production.test")
	compile := exec.Command("go", "test", "-c", "-o", fixture.tests, ".")
	compile.Dir = fixture.app
	if output, err := compile.CombinedOutput(); err != nil {
		return productionFixture{}, fmt.Errorf("compile production handler contracts: %w\n%s", err, output)
	}
	return fixture, nil
}

func requireProductionFixture(t *testing.T) productionFixture {
	t.Helper()
	return requireBuiltFixture(t, builtProductionFixture)
}
func requireOptionsFixture(t *testing.T) productionFixture {
	t.Helper()
	return requireBuiltFixture(t, builtOptionsFixture)
}
func requireBuiltFixture(t *testing.T, build func() (productionFixture, error)) productionFixture {
	t.Helper()
	fixture, err := build()
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

// Begin the long build before joining the parallel-test queue. A targeted unit
// run still builds nothing; only a selected application test starts its fixture.
func startProductionBuild(build func() (productionFixture, error)) func() error {
	return start(func() error { _, err := build(); return err })
}

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
