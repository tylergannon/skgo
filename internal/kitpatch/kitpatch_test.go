package kitpatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tylergannon/skgo/internal/adapter"
)

type fixture struct {
	web string
}

// pnpmDeclaration is how every skgo frontend tells VitePlus which pnpm to run.
const pnpmDeclaration = `"devEngines":{"packageManager":{"name":"pnpm","version":"` + pnpmVersion + `","onFail":"download"}}`

func newFixture(t *testing.T, installed bool) fixture {
	t.Helper()
	web := filepath.Join(t.TempDir(), "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
  "name": "fixture-web",
  "version": "1.0.0",
  "private": true,
  "dependencies": {
    "@sveltejs/kit": "^3.1.0",
    "preserved-runtime": "workspace:*"
  },
  "devDependencies": {
    "preserved-tool": "catalog:"
  },
  "scripts": {"build": "vp build"},
  "custom": {"keep": [1, true, "same"]},
  ` + pnpmDeclaration + `
}
`)
	if err := os.WriteFile(filepath.Join(web, "package.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if installed {
		installStockKit(t, web, "3.1.0", "@sveltejs/kit")
	}
	return fixture{web: web}
}

func (f fixture) options() Options { return Options{Web: f.web} }

func TestConfigureUsesNativePNPMMergeAndIsIdempotent(t *testing.T) {
	f := newFixture(t, true)
	_, patch, err := adapter.KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(f.web, "pnpm-workspace.yaml")
	if err := os.WriteFile(workspace, []byte("packages:\n  - .\ncatalog:\n  preserved-package: ^2.0.0\noverrides:\n  preserved-package: 2.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(f.web, "patches", "absolute.patch")
	if err := writeFixturePatchedDependencies(t, f, map[string]string{
		"preserved-package@1.0.0": "patches/preserved.patch",
		"absolute-package@1.0.0":  absPath,
	}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	opts := f.options()
	opts.Out = &output
	if err := Configure(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Installation is still required") || !strings.Contains(output.String(), f.web) {
		t.Fatalf("apply did not report selected root and required native install: %s", output.String())
	}
	packagePath := filepath.Join(f.web, "package.json")
	gotManifest, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	wantManifest := bytes.Replace([]byte(`{
  "name": "fixture-web",
  "version": "1.0.0",
  "private": true,
  "dependencies": {
    "@sveltejs/kit": "^3.1.0",
    "preserved-runtime": "workspace:*"
  },
  "devDependencies": {
    "preserved-tool": "catalog:"
  },
  "scripts": {"build": "vp build"},
  "custom": {"keep": [1, true, "same"]},
  `+pnpmDeclaration+`
}
`), []byte(`"@sveltejs/kit": "^3.1.0"`), []byte(`"@sveltejs/kit": "3.1.0"`), 1)
	if !bytes.Equal(gotManifest, wantManifest) {
		t.Fatalf("manifest changes escaped the Kit pin:\n%s\nwant:\n%s", gotManifest, wantManifest)
	}
	patchPath := filepath.Join(f.web, "patches", ownedPatchName)
	gotPatch, err := os.ReadFile(patchPath)
	if err != nil || !bytes.Equal(gotPatch, patch) {
		t.Fatalf("copied patch mismatch: %v", err)
	}
	rawPatched, err := readRawPatchedDependencies(f.web)
	if err != nil {
		t.Fatal(err)
	}
	if rawPatched["@sveltejs/kit@3.1.0"] != "patches/skgo-kit-3.1.0-queue.patch" {
		t.Fatalf("authored Kit patch path is not portable: %#v", rawPatched)
	}
	if rawPatched["preserved-package@1.0.0"] != "patches/preserved.patch" || rawPatched["absolute-package@1.0.0"] != absPath {
		t.Fatalf("unrelated authored pnpm patch paths changed: %#v", rawPatched)
	}
	workspaceBytes, err := os.ReadFile(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"packages:", "catalog:", "preserved-package: ^2.0.0", "overrides:"} {
		if !bytes.Contains(workspaceBytes, []byte(field)) {
			t.Errorf("native pnpm config set lost unrelated field %q:\n%s", field, workspaceBytes)
		}
	}
	manifestOnce, patchOnce, workspaceOnce := gotManifest, gotPatch, workspaceBytes
	if err := Configure(opts); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	manifestTwice, _ := os.ReadFile(packagePath)
	patchTwice, _ := os.ReadFile(patchPath)
	workspaceTwice, _ := os.ReadFile(workspace)
	if !bytes.Equal(manifestOnce, manifestTwice) || !bytes.Equal(patchOnce, patchTwice) || !bytes.Equal(workspaceOnce, workspaceTwice) {
		t.Fatal("repeated apply changed project files")
	}
	if err := applyPatchToInstalledKit(t, f, patchPath); err != nil {
		t.Fatal(err)
	}
	if err := Verify(opts); err != nil {
		t.Fatalf("verify corrected installed Kit: %v", err)
	}
}

func TestVerifyRejectsIncompleteConfigurationBeforePackageManager(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"patch", "patch is missing"},
		{"map", "missing"},
		{"installed", "configured but installed Kit could not be verified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, false)
			if err := os.WriteFile(filepath.Join(f.web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"3.1.0"},`+pnpmDeclaration+`}`), 0o644); err != nil {
				t.Fatal(err)
			}
			_, patch, err := adapter.KitQueueCompatibility()
			if err != nil {
				t.Fatal(err)
			}
			if tc.name != "patch" {
				patchPath := filepath.Join(f.web, "patches", ownedPatchName)
				if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name != "map" {
				if err := writeFixturePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@3.1.0": "patches/" + ownedPatchName}); err != nil {
					t.Fatal(err)
				}
			}
			opts := f.options()
			opts.Run = func(string, []string, string, ...string) ([]byte, error) {
				t.Fatal("Verify invoked the package manager before local refusal")
				return nil, nil
			}
			if err := Verify(opts); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Verify = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestConfigureRejectsAmbiguousOrMalformedWorkspaceYAMLBeforeWrites(t *testing.T) {
	cases := map[string]string{
		"duplicate entry key": "packages: [.]\npatchedDependencies:\n  duplicate@1.0.0: patches/one.patch\n  duplicate@1.0.0: patches/two.patch\n",
		"numeric path":        "packages: [.]\npatchedDependencies:\n  package@1.0.0: 17\n",
		"wrong shape":         "packages: [.]\npatchedDependencies: false\n",
		"multiple documents":  "packages: [.]\n---\npackages: [.]\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, false)
			workspace := filepath.Join(f.web, "pnpm-workspace.yaml")
			beforeWorkspace := []byte(contents)
			if err := os.WriteFile(workspace, beforeWorkspace, 0o644); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(f.web, "package.json")
			beforeManifest, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			options := f.options()
			var nativeCalls int
			options.Run = func(string, []string, string, ...string) ([]byte, error) {
				nativeCalls++
				return nil, errors.New("unexpected native pnpm invocation for malformed YAML")
			}
			if err := Configure(options); err == nil {
				t.Fatal("malformed or ambiguous YAML configuration was accepted")
			}
			if nativeCalls != 0 {
				t.Fatalf("malformed YAML invoked pnpm %d times", nativeCalls)
			}
			assertBytesUnchanged(t, workspace, beforeWorkspace)
			assertBytesUnchanged(t, manifestPath, beforeManifest)
			if _, err := os.Stat(filepath.Join(f.web, "patches")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("preflight refusal created patch directory: %v", err)
			}
		})
	}
}

func TestConfigureRefusesNativeAndRawPatchedDependenciesDisagreement(t *testing.T) {
	f := newFixture(t, false)
	workspace := filepath.Join(f.web, "pnpm-workspace.yaml")
	beforeWorkspace := []byte("packages: [.]\npatchedDependencies:\n  authored@1.0.0: patches/authored.patch\n")
	if err := os.WriteFile(workspace, beforeWorkspace, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(f.web, "package.json")
	beforeManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	options := f.options()
	options.Run = func(_ string, _ []string, _ string, args ...string) ([]byte, error) {
		return []byte(`{"external@1.0.0":"/outside/patch.patch"}`), nil
	}
	if err := Configure(options); err == nil || !strings.Contains(err.Error(), "configuration is ambiguous") {
		t.Fatalf("native/raw config disagreement was not refused: %v", err)
	}
	assertBytesUnchanged(t, workspace, beforeWorkspace)
	assertBytesUnchanged(t, manifestPath, beforeManifest)
	if _, err := os.Stat(filepath.Join(f.web, "patches")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ambiguous configuration created patch directory: %v", err)
	}
}

func TestRunPackageManagerForwardsThroughQualifiedVitePlus(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{`+pnpmDeclaration+`,"packageManager":"pnpm@`+pnpmVersion+`+sha512.abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	options := Options{VP: "/qualified/vp", Run: func(_ string, env []string, name string, args ...string) ([]byte, error) {
		if !slices.Equal(env, []string{"pnpm_config_pm_on_fail=ignore"}) {
			t.Errorf("pnpm may manage its own version: env %q", env)
		}
		calls = append(calls, append([]string{name}, args...))
		return []byte("null\n"), nil
	}}
	_, err := runPackageManager(options, dir, "config", "get", "patchedDependencies", "--location=project", "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"/qualified/vp", "pm", "config", "get", "patchedDependencies", "--location=project", "--json"}}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("native read commands = %#v, want %#v", calls, want)
	}
}

func TestRunPackageManagerRefusesAnotherDeclaredPNPM(t *testing.T) {
	for name, manifest := range map[string]string{
		"undeclared":           `{"private":true}`,
		"other devEngines":     `{"devEngines":{"packageManager":{"name":"pnpm","version":"12.10.1"}}}`,
		"other packageManager": `{` + pnpmDeclaration + `,"packageManager":"pnpm@12.10.1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			forwarded := false
			options := Options{Run: func(string, []string, string, ...string) ([]byte, error) {
				forwarded = true
				return nil, nil
			}}
			_, err := runPackageManager(options, dir, "config", "set", "patchedDependencies", "{}", "--location=project", "--json")
			if err == nil || forwarded {
				t.Fatalf("another pnpm was accepted: err=%v forwarded=%t", err, forwarded)
			}
		})
	}
}

func TestConfigureRefusesDuplicateKitJSONKeysWithoutRewritingPackage(t *testing.T) {
	f := newFixture(t, false)
	manifestPath := filepath.Join(f.web, "package.json")
	manifest := []byte(`{"dependencies":{"@sveltejs/kit":"catalog:","@sveltejs/kit":"^3.1.0"}}`)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	options := f.options()
	var nativeCalls int
	options.Run = func(string, []string, string, ...string) ([]byte, error) {
		nativeCalls++
		return nil, errors.New("unexpected native pnpm invocation for duplicate package key")
	}
	if err := Configure(options); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate Kit dependency key was not refused: %v", err)
	}
	if nativeCalls != 0 {
		t.Fatalf("duplicate package JSON invoked pnpm %d times", nativeCalls)
	}
	assertBytesUnchanged(t, manifestPath, manifest)
	if _, err := os.Stat(filepath.Join(f.web, "patches")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("duplicate JSON refusal created patch directory: %v", err)
	}
}

func TestConfigureRefusalsPrecedeAuthoredWrites(t *testing.T) {
	t.Run("unsupported installed version", func(t *testing.T) {
		f := newFixture(t, false)
		installStockKit(t, f.web, "3.0.1", "@sveltejs/kit")
		assertConfigureRefusesWithoutWrites(t, f, "supported package is")
	})
	t.Run("altered installed queue", func(t *testing.T) {
		f := newFixture(t, false)
		installStockKitBytes(t, f.web, "3.1.0", "@sveltejs/kit", []byte("altered source\n"))
		assertConfigureRefusesWithoutWrites(t, f, "unexpected SHA-256")
	})
	t.Run("incompatible exact Kit patch", func(t *testing.T) {
		f := newFixture(t, true)
		if err := writeFixturePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@3.1.0": "patches/other-kit.patch"}); err != nil {
			t.Fatal(err)
		}
		beforeConfig, err := os.ReadFile(filepath.Join(f.web, "pnpm-workspace.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		assertConfigureRefusesWithoutWrites(t, f, "conflicting @sveltejs/kit@3.1.0")
		assertBytesUnchanged(t, filepath.Join(f.web, "pnpm-workspace.yaml"), beforeConfig)
	})
	t.Run("absolute owned patch path", func(t *testing.T) {
		f := newFixture(t, true)
		_, patch, err := adapter.KitQueueCompatibility()
		if err != nil {
			t.Fatal(err)
		}
		patchPath := filepath.Join(f.web, "patches", ownedPatchName)
		if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeFixturePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@3.1.0": patchPath}); err != nil {
			t.Fatal(err)
		}
		assertConfigureRefusesWithoutWrites(t, f, "portable relative path")
	})
	t.Run("wildcard Kit patch", func(t *testing.T) {
		f := newFixture(t, true)
		if err := writeFixturePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@^3": "patches/wildcard.patch"}); err != nil {
			t.Fatal(err)
		}
		assertConfigureRefusesWithoutWrites(t, f, "conflicting Kit patch entry")
	})
	t.Run("different destination bytes", func(t *testing.T) {
		f := newFixture(t, true)
		patchPath := filepath.Join(f.web, "patches", ownedPatchName)
		if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(patchPath, []byte("reviewer-owned patch\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := []byte("reviewer-owned patch\n")
		assertConfigureRefusesWithoutWrites(t, f, "refusing to overwrite different patch bytes")
		assertBytesUnchanged(t, patchPath, before)
	})
	t.Run("parent workspace even when explicitly selected", func(t *testing.T) {
		parent := t.TempDir()
		web := filepath.Join(parent, "web")
		if err := os.MkdirAll(web, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"^3.1.0"},`+pnpmDeclaration+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		parentConfig := filepath.Join(parent, "pnpm-workspace.yaml")
		before := []byte("packages:\n  - web\noverrides:\n  sentinel: 1.2.3\n")
		if err := os.WriteFile(parentConfig, before, 0o644); err != nil {
			t.Fatal(err)
		}
		f := fixture{web: web}
		opts := f.options()
		opts.Workspace = parent
		if err := Configure(opts); err == nil || !strings.Contains(err.Error(), "refusing to modify parent pnpm workspace") {
			t.Fatalf("parent workspace was not refused: %v", err)
		}
		assertBytesUnchanged(t, parentConfig, before)
		assertBytesUnchanged(t, filepath.Join(web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"^3.1.0"},`+pnpmDeclaration+`}`))
		if _, err := os.Stat(filepath.Join(web, "patches")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("refusal created patch directory: %v", err)
		}
	})
	t.Run("selected frontend workspace remains isolated from parent", func(t *testing.T) {
		parent := t.TempDir()
		web := filepath.Join(parent, "web")
		if err := os.MkdirAll(web, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"^3.1.0"},`+pnpmDeclaration+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		parentConfig := filepath.Join(parent, "pnpm-workspace.yaml")
		parentBefore := []byte("packages:\n  - web\noverrides:\n  sentinel: 1.2.3\n")
		if err := os.WriteFile(parentConfig, parentBefore, 0o644); err != nil {
			t.Fatal(err)
		}
		selectedConfig := filepath.Join(web, "pnpm-workspace.yaml")
		selectedBefore := []byte("packages:\n  - .\ncatalog:\n  selected: ^1\n")
		if err := os.WriteFile(selectedConfig, selectedBefore, 0o644); err != nil {
			t.Fatal(err)
		}
		f := fixture{web: web}
		wantRoot, err := filepath.EvalSymlinks(web)
		if err != nil {
			t.Fatal(err)
		}
		selected, err := resolveWeb(f.options())
		if err != nil || selected != wantRoot {
			t.Fatalf("selected root = %q, %v; want %q", selected, err, wantRoot)
		}
		assertBytesUnchanged(t, parentConfig, parentBefore)
		assertBytesUnchanged(t, selectedConfig, selectedBefore)
	})
	t.Run("web symlink cannot hide parent workspace", func(t *testing.T) {
		parent := t.TempDir()
		web := filepath.Join(parent, "web")
		if err := os.MkdirAll(web, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := []byte(`{"dependencies":{"@sveltejs/kit":"^3.1.0"},` + pnpmDeclaration + `}`)
		if err := os.WriteFile(filepath.Join(web, "package.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
		parentConfig := filepath.Join(parent, "pnpm-workspace.yaml")
		parentBefore := []byte("packages:\n  - web\noverrides:\n  sentinel: 1.2.3\n")
		if err := os.WriteFile(parentConfig, parentBefore, 0o644); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(t.TempDir(), "web-alias")
		if err := os.Symlink(web, alias); err != nil {
			t.Fatal(err)
		}
		f := fixture{web: alias}
		if err := Configure(f.options()); err == nil || !strings.Contains(err.Error(), "refusing to modify parent pnpm workspace") {
			t.Fatalf("physical parent workspace was not refused through web symlink: %v", err)
		}
		assertBytesUnchanged(t, parentConfig, parentBefore)
		assertBytesUnchanged(t, filepath.Join(web, "package.json"), manifest)
		if _, err := os.Stat(filepath.Join(web, "patches")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("symlinked parent refusal created patch directory: %v", err)
		}
	})
	t.Run("selected workspace symlink", func(t *testing.T) {
		f := newFixture(t, false)
		outside := filepath.Join(t.TempDir(), "pnpm-workspace.yaml")
		if err := os.WriteFile(outside, []byte("sentinel: true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(f.web, "pnpm-workspace.yaml")); err != nil {
			t.Fatal(err)
		}
		if err := Configure(f.options()); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("selected workspace symlink was not refused: %v", err)
		}
		if _, err := os.Stat(filepath.Join(f.web, "patches")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("workspace symlink refusal created patch directory: %v", err)
		}
	})
}

func TestPinKitChangesOnlyTheExistingDeclaration(t *testing.T) {
	input := []byte("{\n  \"dependencies\": {\n    \"@sveltejs/kit\": \"catalog:\",\n    \"keep\": \"workspace:*\"\n  },\n  \"catalog\": {\n    \"keep\": \"^1\"\n  }\n}\n")
	_, declaration, got, err := pinKit(input, "3.1.0")
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(input, []byte(`"@sveltejs/kit": "catalog:"`), []byte(`"@sveltejs/kit": "3.1.0"`), 1)
	if declaration != "catalog:" || !bytes.Equal(got, want) {
		t.Fatalf("pin changed unrelated package bytes or declaration = %q:\n%s", declaration, got)
	}
	if _, _, _, err := pinKit([]byte(`{"dependencies":{"@sveltejs/kit":"^3"},"devDependencies":{"@sveltejs/kit":"^3"}}`), "3.1.0"); err == nil {
		t.Fatal("duplicate Kit dependency sections were accepted")
	}
	for _, input := range []string{
		`{"dependencies":{"@sveltejs/kit":"catalog:","@sveltejs/kit":"^3.1.0"}}`,
		`{"dependencies":{"@sveltejs/kit":"catalog:"},"dependencies":{"@sveltejs/kit":"^3.1.0"}}`,
	} {
		if _, _, _, err := pinKit([]byte(input), "3.1.0"); err == nil {
			t.Fatalf("duplicate package JSON keys were accepted: %s", input)
		}
	}
}

func assertConfigureRefusesWithoutWrites(t *testing.T, f fixture, wantError string) {
	t.Helper()
	manifestPath := filepath.Join(f.web, "package.json")
	beforeManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(f.web, "pnpm-workspace.yaml")
	beforeConfig, configErr := os.ReadFile(configPath)
	patchPath := filepath.Join(f.web, "patches", ownedPatchName)
	beforePatch, patchErr := os.ReadFile(patchPath)
	options := f.options()
	var nativeCalls int
	options.Run = func(string, []string, string, ...string) ([]byte, error) {
		nativeCalls++
		return nil, errors.New("unexpected native pnpm invocation for local refusal")
	}
	err = Configure(options)
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("Configure error = %v; want substring %q", err, wantError)
	}
	if nativeCalls != 0 {
		t.Fatalf("Configure invoked pnpm %d times before local refusal %q", nativeCalls, wantError)
	}
	assertBytesUnchanged(t, manifestPath, beforeManifest)
	if configErr == nil {
		assertBytesUnchanged(t, configPath, beforeConfig)
	} else if !errors.Is(configErr, os.ErrNotExist) {
		t.Fatal(configErr)
	} else if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight refusal created pnpm config: %v", err)
	}
	if patchErr == nil {
		assertBytesUnchanged(t, patchPath, beforePatch)
	} else if !errors.Is(patchErr, os.ErrNotExist) {
		t.Fatal(patchErr)
	} else if _, err := os.Stat(patchPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight refusal created the patch destination: %v", err)
	}
}

// The installed pin is immutable input. Recover its stock queue once, then
// copy those bytes into each fixture rather than launching git per assertion.
var stockKitQueue = sync.OnceValues(func() ([]byte, error) {
	stockDir := filepath.Join("..", "..", "example", "web", "node_modules", "@sveltejs", "kit")
	stockManifest, err := os.ReadFile(filepath.Join(stockDir, "package.json"))
	if err != nil {
		return nil, fmt.Errorf("installed pinned Kit source is required: %w", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(stockManifest, &manifest); err != nil {
		return nil, err
	}
	if manifest["name"] != kitPackageName || manifest["version"] != "3.1.0" {
		return nil, fmt.Errorf("authoritative installed Kit identity = %v@%v", manifest["name"], manifest["version"])
	}
	queue, err := os.ReadFile(filepath.Join(stockDir, "src", "core", "postbuild", "queue.js"))
	if err != nil {
		return nil, err
	}
	metadata, patch, err := adapter.KitQueueCompatibility()
	if err != nil {
		return nil, err
	}
	if digest(queue) == metadata.CorrectedSHA256 {
		root, err := os.MkdirTemp("", "skgo-stock-kit-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(root)
		queuePath := filepath.Join(root, filepath.FromSlash(metadata.QueuePath))
		if err := os.MkdirAll(filepath.Dir(queuePath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(queuePath, queue, 0o644); err != nil {
			return nil, err
		}
		patchPath := filepath.Join(root, "queue.patch")
		if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
			return nil, err
		}
		cmd := exec.Command("git", "apply", "--reverse", patchPath)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("recover stock queue: %w: %s", err, output)
		}
		queue, err = os.ReadFile(queuePath)
		if err != nil {
			return nil, err
		}
	}
	if digest(queue) != metadata.StockSHA256 {
		return nil, fmt.Errorf("installed Kit queue source hash %s is neither packaged stock %s nor corrected %s", digest(queue), metadata.StockSHA256, metadata.CorrectedSHA256)
	}
	return queue, nil
})

func installStockKit(t *testing.T, web, version, name string) {
	t.Helper()
	queue, err := stockKitQueue()
	if err != nil {
		t.Fatal(err)
	}
	installStockKitBytes(t, web, version, name, queue)
}

func installStockKitBytes(t *testing.T, web, version, name string, queue []byte) {
	t.Helper()
	kitDir := filepath.Join(web, "node_modules", "@sveltejs", "kit")
	queuePath := filepath.Join(kitDir, "src", "core", "postbuild", "queue.js")
	if err := os.MkdirAll(filepath.Dir(queuePath), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]string{"name": name, "version": version})
	if err := os.WriteFile(filepath.Join(kitDir, "package.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(queuePath, queue, 0o644); err != nil {
		t.Fatal(err)
	}
}

func applyPatchToInstalledKit(t *testing.T, f fixture, patchPath string) error {
	t.Helper()
	kitDir := filepath.Join(f.web, "node_modules", "@sveltejs", "kit")
	cmd := exec.Command("git", "apply", patchPath)
	cmd.Dir = kitDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply packaged patch to disposable installed Kit fixture: %w: %s", err, output)
	}
	metadata, _, err := adapter.KitQueueCompatibility()
	if err != nil {
		return err
	}
	queue, err := os.ReadFile(filepath.Join(kitDir, filepath.FromSlash(metadata.QueuePath)))
	if err != nil {
		return err
	}
	if digest(queue) != metadata.CorrectedSHA256 {
		return fmt.Errorf("patched fixture queue digest is %s; expected %s", digest(queue), metadata.CorrectedSHA256)
	}
	return nil
}

func writeFixturePatchedDependencies(t *testing.T, f fixture, values map[string]string) error {
	t.Helper()
	jsonValue, err := json.Marshal(values)
	if err != nil {
		return err
	}
	path := filepath.Join(f.web, "pnpm-workspace.yaml")
	before, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, append(before, []byte("patchedDependencies: "+string(jsonValue)+"\n")...), 0o644)
}

func assertBytesUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s changed or became unreadable: %v\n got %q\nwant %q", path, err, got, want)
	}
}
