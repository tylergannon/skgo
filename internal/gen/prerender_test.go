package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const loadSource = `package routes

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Data struct { Message string ` + "`json:\"message\"`" + ` }

func site(context.Context) (Data, error) { return Data{Message: "hello"}, nil }

var _ = skgo.Load(site)
`

func TestPrerenderGoLoadFailureNamesAuthoredRoute(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, filepath.Join(t.TempDir(), "example"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(app, "web"), filepath.Join(app, "ui")); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(app, "internal", "skgo", "config.go")
	configSource, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	updatedConfig := strings.Replace(string(configSource), "--web ../../web", "--web ../../ui", 1)
	if updatedConfig == string(configSource) {
		t.Fatal("could not point generation at the fixture's ui/ frontend")
	}
	if err := os.WriteFile(config, []byte(updatedConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(app, "go.mod")
	goModSource, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	updatedGoMod := strings.Replace(string(goModSource), "ignore ./web/node_modules", "ignore ./ui/node_modules", 1)
	if err := os.WriteFile(goMod, []byte(updatedGoMod), 0o644); err != nil {
		t.Fatal(err)
	}
	linkExampleFrontendDependencies(t, root, app)

	const failure = "prerender fixture literal failure"
	layout := filepath.Join(app, "ui", "src", "routes", "layout.server.go")
	source, err := os.ReadFile(layout)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(source), "\t\"context\"", "\t\"context\"\n\t\"errors\"", 1)
	broken = strings.Replace(broken, "func layoutLoad(ctx context.Context) (RootLayoutData, error) {", "func layoutLoad(ctx context.Context) (RootLayoutData, error) {\n\treturn RootLayoutData{}, errors.New(\""+failure+"\")", 1)
	if broken == string(source) || !strings.Contains(broken, failure) {
		t.Fatal("failed to install the literal Go load failure in the fixture")
	}
	if err := os.WriteFile(layout, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("go generate ./...: %v\n%s", err, out)
	}

	cmd := exec.Command(filepath.Join(app, "ui", "node_modules", ".bin", "vp"), "build")
	cmd.Dir = filepath.Join(app, "ui")
	cmd.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	output, buildErr := cmd.CombinedOutput()
	if buildErr == nil {
		t.Fatalf("vp build succeeded despite the Go layout load failure:\n%s", output)
	}
	got := string(output)
	for _, want := range []string{
		"route ID /about",
		"path /about",
		"source src/routes/layout.server.go",
		failure,
		"GET /about",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("failed frontend build output does not contain %q:\n%s", want, got)
		}
	}
}

func linkExampleFrontendDependencies(t *testing.T, root, app string) {
	t.Helper()
	source := filepath.Join(root, "example", "web", "node_modules")
	if _, err := os.Stat(filepath.Join(source, ".bin", "vp")); err != nil {
		t.Fatalf("example web dependencies are missing, so the real vp build cannot run: %v; install the pinned dependencies first", err)
	}
	destination := filepath.Join(app, "ui", "node_modules")
	if err := os.MkdirAll(filepath.Join(destination, "@skgo"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "@skgo" || entry.Name() == "$app" || entry.Name() == ".vite-temp" {
			continue
		}
		if err := os.Symlink(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			t.Fatalf("link pinned frontend dependency %s: %v", entry.Name(), err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "internal", "adapter"), filepath.Join(destination, "@skgo", "sveltekit-adapter")); err != nil {
		t.Fatalf("link current skgo adapter: %v", err)
	}
}

func TestAPrerenderedPageCanHaveAGoLayoutLoadInItsBranch(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/layout.server.go":   loadSource,
		"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":     "export const prerender = true;\n",
	})

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if command := readFixtureFile(t, root, "app/generated/prerender/main_gen.go"); !strings.Contains(command, "nil, generated.Loads()") {
		t.Fatalf("build command omits generated loads:\n%s", command)
	}
}

func TestPrerenderInheritanceMatchesKit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "a leaf inherits true from its layout",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/+layout.ts":         "export const prerender = true;\n",
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
			},
		},
		{
			name: "a leaf false overrides a layout true",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/+layout.ts":         "export const prerender = true;\n",
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
				"app/web/src/routes/about/+page.ts":     "export const prerender = false;\n",
			},
		},
		{
			name: "a leaf true overrides a layout false",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/+layout.ts":         "export const prerender = false;\n",
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
				"app/web/src/routes/about/+page.ts":     "export const prerender = true;\n",
			},
		},
		{
			name: "a universal option wins over the same node server option",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":         loadSource,
				"app/web/src/routes/branch/+layout.ts":        "export const prerender = false;\n",
				"app/web/src/routes/branch/+layout.server.ts": "export const prerender = true;\n",
				"app/web/src/routes/branch/page/+page.svelte": "<h1>Page</h1>\n",
			},
		},
		{
			name: "auto can enter kits prerender crawl",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":   loadSource,
				"app/web/src/routes/about/+page.svelte": "<h1>About</h1>\n",
				"app/web/src/routes/about/+page.ts":     "export const prerender: boolean | 'auto' = 'auto';\n",
			},
		},
		{
			name: "an unknown universal option does not fall back to the server option",
			files: map[string]string{
				"app/web/src/routes/layout.server.go":         loadSource,
				"app/web/src/routes/branch/+layout.ts":        "const choice = false;\nexport const prerender = choice;\n",
				"app/web/src/routes/branch/+layout.server.ts": "export const prerender = true;\n",
				"app/web/src/routes/branch/page/+page.svelte": "<h1>Page</h1>\n",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, cfg := foreignFixture(t, "", test.files)
			if err := Run(cfg); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	}
}

func TestLoadsOutsideThePrerenderedBranchAreAccepted(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/private/layout.server.go": loadSource,
		"app/web/src/routes/private/+page.svelte":     "<h1>Private</h1>\n",
		"app/web/src/routes/about/+page.svelte":       "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":           "export const prerender = true;\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAPrerenderedPageCanHaveItsOwnGoLoad(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/about/page.server.go": loadSource,
		"app/web/src/routes/about/+page.svelte":   "<h1>About</h1>\n",
		"app/web/src/routes/about/+page.ts":       "export const prerender = true;\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestANamedLayoutResetExcludesLoadsOutsideKitsBranch(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/+layout.svelte":            "<slot />\n",
		"app/web/src/routes/+layout.ts":                "export const prerender = true;\n",
		"app/web/src/routes/branch/+layout.svelte":     "<slot />\n",
		"app/web/src/routes/branch/layout.server.go":   loadSource,
		"app/web/src/routes/branch/page/+page@.svelte": "<h1>Page</h1>\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestTheLoadStubBridgesAPrerenderCall(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/account/page.server.go": loadSource,
		"app/web/src/routes/account/+page.svelte":   "<h1>Account</h1>\n",
	})
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stub := readFixtureFile(t, root, "app/web/src/routes/account/+page.server.ts")
	for _, want := range []string{
		"import { building } from '$app/env'",
		"skgoPrerenderLoad",
		`buildLoad("src/routes/account/+page.server.ts", "src/routes/account/page.server.go", event)`,
		"event.url.pathname",
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("stub does not contain %q:\n%s", want, stub)
		}
	}
}

func TestOptionExamplesInCommentsAndStringsAreIgnored(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"// export const prerender = true;\nexport const nope = false;\n",
		"const example = 'export const prerender = true';\n",
		"/* export const prerender = 'auto'; */\n",
	} {
		if value, ok, err := sourcePrerender(writeOptionFixture(t, source)); err != nil || ok {
			t.Fatalf("sourcePrerender = %q, %v, %v; want no option", value, ok, err)
		}
	}
}

func writeOptionFixture(t *testing.T, source string) string {
	t.Helper()
	path := t.TempDir() + "/+page.ts"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
