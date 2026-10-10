package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"gopkg.in/yaml.v3"
)

const qualifiedVPGraph = `{"schemaVersion":1,"source":{"scope":"global","vitePlusVersion":"1.0.0"},"nodes":[{"name":"vite","version":"8.3.1"},{"name":"@voidzero-dev/vite-plus-core","version":"1.0.0"},{"name":"vitest","version":"5.0.1"}]}`

func TestQualifiedVPDependencyVersions(t *testing.T) {
	got, err := parseVPToolchain([]byte(qualifiedVPGraph))
	if err != nil || got != (vpDependencies{Core: "1.0.0", Vitest: "5.0.1"}) {
		t.Fatalf("qualified dependencies = %+v, %v", got, err)
	}
	for _, tc := range []struct{ name, old, replacement string }{
		{"wrong CLI", `"vitePlusVersion":"1.0.0"`, `"vitePlusVersion":"1.0.1"`},
		{"local CLI", `"scope":"global"`, `"scope":"local"`},
		{"unknown schema", `"schemaVersion":1`, `"schemaVersion":2`},
		{"missing core", `"@voidzero-dev/vite-plus-core"`, `"not-core"`},
		{"missing Vitest", `"name":"vitest"`, `"name":"not-vitest"`},
		{"invalid pin", `"version":"5.0.1"`, `"version":"latest"`},
		{"duplicate pin", `"name":"vite"`, `"name":"vitest"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseVPToolchain([]byte(strings.Replace(qualifiedVPGraph, tc.old, tc.replacement, 1))); err == nil {
				t.Fatal("accepted unqualified toolchain graph")
			}
		})
	}
}

func TestExistingVitePlusDetection(t *testing.T) {
	for _, tc := range []struct {
		manifest string
		want     bool
	}{
		{`{"devDependencies":{"vite-plus":"catalog:"}}`, true},
		{`{"dependencies":{"vite-plus":"0.9.0"}}`, true},
		{`{"devDependencies":{"vite":"8.0.0"},"scripts":{"build":"vp build"}}`, false},
	} {
		root := t.TempDir()
		writeAlignmentFile(t, root, "package.json", tc.manifest)
		got, err := usesVitePlus(root)
		if err != nil || got != tc.want {
			t.Fatalf("detect %s: %v, %v", tc.manifest, got, err)
		}
	}
}

func TestAlignmentPreservesAuthoredFilesAndUnrelatedSelections(t *testing.T) {
	for _, layout := range []string{"direct", "catalog", "named-catalog", "package-overrides", "package-config"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			pkg := map[string]any{
				"name":                 "authored-app",
				"scripts":              map[string]any{"test": "pnpm run test:unit --run", "preview": "cd .. && just serve"},
				"devEngines":           map[string]any{"runtime": map[string]any{"name": "node", "version": "24.0.0"}},
				"optionalDependencies": map[string]any{"vitest": "4.1.0", "optional-app": "2.3.4"},
				"peerDependencies":     map[string]any{"vite": "npm:@voidzero-dev/vite-plus-core@0.9.0", "peer-app": "2.3.4"},
				"devDependencies": map[string]any{
					"vite-plus": "0.9.0", "vite": "npm:@voidzero-dev/vite-plus-core@0.9.0", "vitest": "4.1.0",
					"@vitest/browser-playwright": "4.1.0", "@vitest/browser-webdriverio": "5.0.0", "@vitest/eslint-plugin": "1.3.4", "vitest-browser-svelte": "3.1.0", "application-library": "2.3.4",
				},
			}
			workspace := "# Preserve the reason for this application override.\noverrides:\n  application-library: 2.3.4 # Do not float this dependency.\nallowBuilds:\n  esbuild: true\n"
			if layout == "catalog" {
				pkg["devDependencies"].(map[string]any)["vite"] = "catalog:"
				pkg["devDependencies"].(map[string]any)["vite-plus"] = "catalog:"
				workspace += "catalog:\n  vite: npm:@voidzero-dev/vite-plus-core@0.9.0\n  vite-plus: 0.9.0 # Align this shared vp version.\n  vitest: 4.1.0\n  application-library: 2.3.4\n"
			}
			if layout == "named-catalog" {
				pkg["devDependencies"].(map[string]any)["vite"] = "catalog:tools"
				pkg["devDependencies"].(map[string]any)["vite-plus"] = "catalog:tools"
				workspace += "catalogs:\n  tools:\n    vite: npm:@voidzero-dev/vite-plus-core@0.9.0\n    vite-plus: 0.9.0 # Align this shared vp version.\n    vitest: 4.1.0\n    application-library: 2.3.4\n"
			}
			if layout == "package-overrides" {
				pkg["pnpm"] = map[string]any{"overrides": map[string]any{"vitest": "4.1.0", "vite@*": "npm:@voidzero-dev/vite-plus-core@0.9.0", "application-library": "2.3.4"}}
			}
			if layout == "package-config" {
				pkg["pnpm"] = map[string]any{"onlyBuiltDependencies": []string{"esbuild"}}
			}
			data, err := json.Marshal(pkg)
			if err != nil {
				t.Fatal(err)
			}
			writeAlignmentFile(t, root, "package.json", string(data))
			writeAlignmentFile(t, root, "pnpm-workspace.yaml", workspace)
			writeAlignmentFile(t, root, "src/receipt.ts", "// authored formatting must remain\nexport const receipt = 'kept' ;\n")
			before, err := frontendFiles(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := alignVPDependencies(root, vpDependencies{Core: "1.0.0", Vitest: "5.0.1"}); err != nil {
				t.Fatal(err)
			}
			aligned := readAlignmentJSON(t, filepath.Join(root, "package.json"))
			deps := aligned["devDependencies"].(map[string]any)
			wantVP, wantVite := "1.0.0", "npm:@voidzero-dev/vite-plus-core@1.0.0"
			if layout == "catalog" {
				wantVP, wantVite = "catalog:", "catalog:"
			}
			if layout == "named-catalog" {
				wantVP, wantVite = "catalog:tools", "catalog:tools"
			}
			for key, want := range map[string]string{"vite-plus": wantVP, "vite": wantVite, "vitest": "5.0.1", "@vitest/browser-playwright": "5.0.1", "@vitest/browser-webdriverio": "5.0.0", "@vitest/eslint-plugin": "1.3.4", "vitest-browser-svelte": "3.1.0", "application-library": "2.3.4"} {
				if deps[key] != want {
					t.Errorf("%s = %v; want %s", key, deps[key], want)
				}
			}
			if !reflect.DeepEqual(aligned["scripts"], pkg["scripts"]) || !reflect.DeepEqual(aligned["devEngines"], pkg["devEngines"]) {
				t.Fatalf("authored manifest configuration changed: %v", aligned)
			}
			if !reflect.DeepEqual(aligned["optionalDependencies"], map[string]any{"vitest": "5.0.1", "optional-app": "2.3.4"}) || !reflect.DeepEqual(aligned["peerDependencies"], map[string]any{"vite": "npm:@voidzero-dev/vite-plus-core@1.0.0", "peer-app": "2.3.4"}) {
				t.Fatalf("optional/peer selections were not preserved and aligned: %v", aligned)
			}
			config := readAlignmentYAML(t, filepath.Join(root, "pnpm-workspace.yaml"))
			workspaceBytes, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, comment := range []string{"# Preserve the reason for this application override.", "# Do not float this dependency."} {
				if !bytes.Contains(workspaceBytes, []byte(comment)) {
					t.Errorf("alignment lost authored comment %q: %s", comment, workspaceBytes)
				}
			}
			// The qualified pnpm 12.9.1 ignores package.json's legacy pnpm
			// field. The executable-level test below verifies this boundary.
			overrides := config["overrides"].(map[string]any)
			if layout == "package-overrides" {
				legacy := aligned["pnpm"].(map[string]any)["overrides"].(map[string]any)
				if legacy["application-library"] != "2.3.4" {
					t.Fatalf("legacy unrelated override was changed: %v", legacy)
				}
			}
			if overrides["vitest@*"] != "5.0.1" {
				t.Fatalf("missing effective Vitest override: %v", overrides)
			}
			if overrides["application-library"] != "2.3.4" || config["allowBuilds"].(map[string]any)["esbuild"] != true {
				t.Fatalf("unrelated effective overrides/build configuration changed: overrides=%v, workspace=%v", overrides, config)
			}
			if layout == "catalog" || layout == "named-catalog" {
				if !bytes.Contains(workspaceBytes, []byte("# Align this shared vp version.")) {
					t.Errorf("lost comment on an aligned scalar: %s", workspaceBytes)
				}
				catalog, _ := config["catalog"].(map[string]any)
				if layout == "named-catalog" {
					catalog = config["catalogs"].(map[string]any)["tools"].(map[string]any)
				}
				if !reflect.DeepEqual(catalog, map[string]any{"vite": "npm:@voidzero-dev/vite-plus-core@1.0.0", "vite-plus": "1.0.0", "vitest": "5.0.1", "application-library": "2.3.4"}) {
					t.Fatalf("catalog alignment: %v", catalog)
				}
			}
			if err := preservedFiles(before, root); err != nil {
				t.Fatalf("dependency-only alignment failed the authored-file guard: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(root, "src/receipt.ts")); err != nil || string(got) != "// authored formatting must remain\nexport const receipt = 'kept' ;\n" {
				t.Fatalf("authored source changed: %q, %v", got, err)
			}
		})
	}
}

func TestAlignmentPublishesOverridesConsumedByQualifiedPNPM(t *testing.T) {
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		t.Fatal("qualified pnpm 12.9.1 is required:", err)
	}
	version, err := exec.Command(pnpm, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "12.9.1" {
		t.Fatalf("qualified pnpm version = %q, %v; want 12.9.1", version, err)
	}
	for _, existingWorkspace := range []bool{false, true} {
		root := t.TempDir()
		writeAlignmentFile(t, root, "package.json", `{"devDependencies":{"vite-plus":"0.9.0"},"pnpm":{"overrides":{"vitest@*":"4.1.0","ignored-legacy-selection":"7.8.9"}}}`)
		want := map[string]string{"vitest@*": "5.0.1"}
		if existingWorkspace {
			writeAlignmentFile(t, root, "pnpm-workspace.yaml", "overrides:\n  application-library: 2.3.4\n")
			want["application-library"] = "2.3.4"
		}
		if err := alignVPDependencies(root, vpDependencies{Core: "1.0.0", Vitest: "5.0.1"}); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(pnpm, "config", "get", "overrides", "--json")
		cmd.Dir = root
		data, err := cmd.Output()
		if err != nil {
			t.Fatalf("read actual pnpm override selection: %v", err)
		}
		var overrides map[string]string
		if err := json.Unmarshal(data, &overrides); err != nil {
			t.Fatalf("pnpm config output %q: %v", data, err)
		}
		if !reflect.DeepEqual(overrides, want) {
			t.Fatalf("qualified pnpm consumes the wrong overrides: %v", overrides)
		}
	}
}

func TestAlignmentDoesNotExemptUnrelatedNestedConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, file, before, after string }{
		{"optional dependency", "package.json", `{"optionalDependencies":{"application-library":"1.0.0"}}`, `{"optionalDependencies":{"application-library":"2.0.0"}}`},
		{"peer dependency", "package.json", `{"peerDependencies":{"application-library":"1.0.0"}}`, `{"peerDependencies":{"application-library":"2.0.0"}}`},
		{"pnpm override", "package.json", `{"pnpm":{"overrides":{"application-library":"1.0.0"}}}`, `{"pnpm":{"overrides":{"application-library":"2.0.0"}}}`},
		{"legacy pnpm Vitest override", "package.json", `{"pnpm":{"overrides":{"vitest":"4.1.0"}}}`, `{"pnpm":{"overrides":{"vitest":"5.0.1"}}}`},
		{"independent Vitest plugin", "package.json", `{"devDependencies":{"@vitest/eslint-plugin":"1.3.4"}}`, `{"devDependencies":{"@vitest/eslint-plugin":"5.0.1"}}`},
		{"named catalog", "pnpm-workspace.yaml", "catalogs:\n  tools:\n    application-library: 1.0.0\n", "catalogs:\n  tools:\n    application-library: 2.0.0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, stage := t.TempDir(), t.TempDir()
			writeAlignmentFile(t, source, tc.file, tc.before)
			writeAlignmentFile(t, stage, tc.file, tc.after)
			if err := preservedFrontend(source, stage); err == nil || !strings.Contains(err.Error(), "authored configuration") {
				t.Fatalf("unrelated configuration change accepted: %v", err)
			}
		})
	}
}

func TestCompletionUsesDependencyAlignmentForExistingVP(t *testing.T) {
	root := t.TempDir()
	writeAlignmentFile(t, root, "go.mod", "module example.com/alignment\n\ngo 1.27.1\nrequire github.com/tylergannon/skgo v0.27.0\n")
	writeAlignmentFile(t, root, "web/package.json", `{"scripts":{"test":"pnpm run test:unit --run"},"devDependencies":{"vite-plus":"catalog:","vite":"catalog:","vitest":"4.1.0","application-library":"2.3.4"}}`)
	writeAlignmentFile(t, root, "web/pnpm-workspace.yaml", "# Keep the shared toolchain in the catalog.\ncatalog:\n  vite-plus: 0.9.0\n  vite: npm:@voidzero-dev/vite-plus-core@0.9.0\n")
	writeAlignmentFile(t, root, "web/src/receipt.ts", "export const receipt = 'preserve me' ;\n")
	before, err := frontendFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop before package-manager execution")
	stage, inspected := "", false
	err = Run(context.Background(), Options{
		Root: root, VP: "/qualified/vp", CompleteVersion: "v0.29.0",
		info:     func() (buildinfo.Info, error) { return buildinfo.Info{SkgoVersion: "v0.29.0"}, nil },
		packages: func(string) (string, string, error) { return "0.28.0", "0.28.0", nil },
		run: func(_ context.Context, c command) ([]byte, error) {
			if c.Name != "/qualified/vp" {
				t.Fatalf("consumer mutation or unexpected executable before staged verification: %+v", c)
			}
			if reflect.DeepEqual(c.Args, []string{"--version"}) {
				return []byte("vp v1.0.0\n"), nil
			}
			if reflect.DeepEqual(c.Args, []string{"toolchain", "--global", "--json", "vite", "vitest"}) {
				return []byte(qualifiedVPGraph), nil
			}
			if reflect.DeepEqual(c.Args, []string{"env", "exec", "--package-manager", "pnpm@12.9.1", "which", "pnpm"}) {
				stage = c.Dir
				if stage == root || stage == filepath.Join(root, "web") {
					t.Fatal("alignment ran in the consumer")
				}
				pkg := readAlignmentJSON(t, filepath.Join(stage, "package.json"))
				deps := pkg["devDependencies"].(map[string]any)
				if deps["vite"] != "catalog:" || deps["vitest"] != "5.0.1" || deps["vite-plus"] != "catalog:" || deps["application-library"] != "2.3.4" || pkg["scripts"].(map[string]any)["test"] != "pnpm run test:unit --run" {
					t.Fatalf("completion did not preserve and align stage: %v", pkg)
				}
				config := readAlignmentYAML(t, filepath.Join(stage, "pnpm-workspace.yaml"))
				if !reflect.DeepEqual(config["catalog"], map[string]any{"vite-plus": "1.0.0", "vite": "npm:@voidzero-dev/vite-plus-core@1.0.0"}) {
					t.Fatalf("completion did not align the referenced catalog: %v", config)
				}
				inspected = true
				return nil, stop
			}
			t.Fatalf("existing vp project attempted migration or an unexpected step: %+v", c)
			return nil, nil
		},
	})
	if !errors.Is(err, stop) || !inspected {
		t.Fatalf("completion did not reach aligned dependency staging: %v, inspected=%v", err, inspected)
	}
	after, err := frontendFiles(root)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed staging mutated consumer: %v", err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("failed update retained temporary frontend stage: %v", err)
	}
}

func writeAlignmentFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func readAlignmentJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func readAlignmentYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := yaml.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
