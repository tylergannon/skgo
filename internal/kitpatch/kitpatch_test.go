package kitpatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/adapter"
)

type fixture struct {
	web  string
	pnpm string
}

func newFixture(t *testing.T, installed bool) fixture {
	t.Helper()
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		t.Fatalf("native pnpm 12.9.1 is required for config behavior tests: %v", err)
	}
	version, err := exec.Command(pnpm, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(version)) != pnpmVersion {
		t.Fatalf("native pnpm version = %q, %v; want %s", strings.TrimSpace(string(version)), err, pnpmVersion)
	}
	web := filepath.Join(t.TempDir(), "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{
  "name": "fixture-web",
  "version": "1.0.0",
  "private": true,
  "dependencies": {
    "@sveltejs/kit": "^3.0.0",
    "preserved-runtime": "workspace:*"
  },
  "devDependencies": {
    "preserved-tool": "catalog:"
  },
  "scripts": {"build": "vp build"},
  "custom": {"keep": [1, true, "same"]}
}
`)
	if err := os.WriteFile(filepath.Join(web, "package.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if installed {
		installStockKit(t, web, "3.0.0", "@sveltejs/kit")
	}
	return fixture{web: web, pnpm: pnpm}
}

func (f fixture) options() Options { return Options{Web: f.web, PNPM: f.pnpm} }

func TestConfigureUsesNativePNPMMergeAndIsIdempotent(t *testing.T) {
	f := newFixture(t, true)
	metadata, patch, err := adapter.KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(f.web, "pnpm-workspace.yaml")
	if err := os.WriteFile(workspace, []byte("packages:\n  - .\ncatalog:\n  preserved-package: ^2.0.0\noverrides:\n  preserved-package: 2.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(f.web, "patches", "absolute.patch")
	if err := setNativePatchedDependencies(t, f, map[string]string{
		"preserved-package@1.0.0": "patches/preserved.patch",
		"absolute-package@1.0.0":  absPath,
	}); err != nil {
		t.Fatal(err)
	}
	beforeWorkspace, err := os.ReadFile(workspace)
	if err != nil {
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
    "@sveltejs/kit": "^3.0.0",
    "preserved-runtime": "workspace:*"
  },
  "devDependencies": {
    "preserved-tool": "catalog:"
  },
  "scripts": {"build": "vp build"},
  "custom": {"keep": [1, true, "same"]}
}
`), []byte(`"@sveltejs/kit": "^3.0.0"`), []byte(`"@sveltejs/kit": "3.0.0"`), 1)
	if !bytes.Equal(gotManifest, wantManifest) {
		t.Fatalf("manifest changes escaped the Kit pin:\n%s\nwant:\n%s", gotManifest, wantManifest)
	}
	patchPath := filepath.Join(f.web, "patches", ownedPatchName)
	gotPatch, err := os.ReadFile(patchPath)
	if err != nil || !bytes.Equal(gotPatch, patch) {
		t.Fatalf("copied patch mismatch: %v", err)
	}
	if digest(gotPatch) != metadata.PatchSHA256 {
		t.Fatalf("patch sha256 = %s, want metadata %s", digest(gotPatch), metadata.PatchSHA256)
	}
	patched := getNativePatchedDependencies(t, f)
	if !sameConfiguredPath(f.web, patched[patchKey(&project{metadata: metadata})], patchPath) {
		t.Fatalf("native project patch entry = %q, want %s", patched[patchKey(&project{metadata: metadata})], patchPath)
	}
	rawPatched, err := readRawPatchedDependencies(f.web)
	if err != nil {
		t.Fatal(err)
	}
	if rawPatched[patchKey(&project{metadata: metadata})] != relativePatchValue(&project{root: f.web, patchPath: patchPath}) {
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
	if !bytes.Contains(beforeWorkspace, []byte("preserved-package: ^2.0.0")) {
		t.Fatal("test did not seed unrelated workspace configuration")
	}
	if err := applyPatchToInstalledKit(t, f, patchPath); err != nil {
		t.Fatal(err)
	}
	if err := Verify(opts); err != nil {
		t.Fatalf("verify corrected installed Kit: %v", err)
	}
	if err := os.Remove(patchPath); err != nil {
		t.Fatal(err)
	}
	if err := Verify(opts); err == nil || !strings.Contains(err.Error(), "patch is missing") {
		t.Fatalf("check accepted a missing canonical patch with corrected installed bytes: %v", err)
	}
	if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setNativePatchedDependencies(t, f, map[string]string{
		"preserved-package@1.0.0": "patches/preserved.patch",
		"absolute-package@1.0.0":  absPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := Verify(opts); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("check accepted missing project map entry with corrected installed bytes: %v", err)
	}
	if err := Configure(opts); err != nil {
		t.Fatalf("restore owned patch config: %v", err)
	}
	if err := Verify(opts); err != nil {
		t.Fatalf("verify after restoring project map entry: %v", err)
	}
}

func TestConfigureConfiguredButUninstalledFailsCheck(t *testing.T) {
	f := newFixture(t, false)
	if err := Configure(f.options()); err != nil {
		t.Fatal(err)
	}
	if err := Verify(f.options()); err == nil || !strings.Contains(err.Error(), "configured but installed Kit could not be verified") {
		t.Fatalf("check accepted configured-but-uninstalled project: %v", err)
	}
}

func TestConfigurePreservesLogicalPatchPathsAcrossYAMLSyntax(t *testing.T) {
	f := newFixture(t, true)
	absPath := filepath.Join(f.web, "patches", "absolute.patch")
	workspace := filepath.Join(f.web, "pnpm-workspace.yaml")
	contents := fmt.Sprintf("packages: [.] # flow list and comment\noverrides: &priorPatches\n  'relative-package@1.0.0': 'patches/relative.patch'\npatchedDependencies:\n  <<: *priorPatches\n  \"absolute-package@1.0.0\": %q # quoted absolute value\ncatalog: {kept: ^1} # flow mapping\n", absPath)
	if err := os.WriteFile(workspace, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(f.options()); err != nil {
		t.Fatalf("configure valid YAML anchors/merge, quotes, flow values, and comments: %v", err)
	}
	raw, err := readRawPatchedDependencies(f.web)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _, err := adapter.KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"relative-package@1.0.0":                  "patches/relative.patch",
		"absolute-package@1.0.0":                  absPath,
		metadata.Package + "@" + metadata.Version: "patches/" + ownedPatchName,
	}
	for key, value := range want {
		if raw[key] != value {
			t.Errorf("authored patchedDependencies[%q] = %q, want original logical string %q; all=%#v", key, raw[key], value, raw)
		}
	}
	if err := Verify(f.options()); err == nil || !strings.Contains(err.Error(), "expected corrected") {
		t.Fatalf("check accepted a stock installed Kit: %v", err)
	}
}

func TestConfigureAcceptsDirectPatchMapAliasAndFlowStyleMap(t *testing.T) {
	for name, workspaceText := range map[string]string{
		"direct alias":         "packages: [.]\noverrides: &priorPatches {relative-package@1.0.0: patches/relative.patch}\npatchedDependencies: *priorPatches\n",
		"flow style patch map": "packages: [.]\npatchedDependencies: {'relative-package@1.0.0': 'patches/relative.patch', \"another-package@2.0.0\": \"patches/another.patch\"} # inline comment\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, true)
			workspace := filepath.Join(f.web, "pnpm-workspace.yaml")
			if err := os.WriteFile(workspace, []byte(workspaceText), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Configure(f.options()); err != nil {
				t.Fatalf("configure valid patch map syntax: %v", err)
			}
			raw, err := readRawPatchedDependencies(f.web)
			if err != nil {
				t.Fatal(err)
			}
			if raw["relative-package@1.0.0"] != "patches/relative.patch" {
				t.Fatalf("relative patch source string changed: %#v", raw)
			}
			if name == "flow style patch map" && raw["another-package@2.0.0"] != "patches/another.patch" {
				t.Fatalf("flow-style patch map was not preserved: %#v", raw)
			}
		})
	}
}

func TestConfigureRejectsAmbiguousOrMalformedWorkspaceYAMLBeforeWrites(t *testing.T) {
	cases := map[string]string{
		"duplicate entry key":     "packages: [.]\npatchedDependencies:\n  duplicate@1.0.0: patches/one.patch\n  duplicate@1.0.0: patches/two.patch\n",
		"duplicate top-level map": "packages: [.]\npatchedDependencies: {}\npatchedDependencies: {}\n",
		"non-string patch key":    "packages: [.]\npatchedDependencies:\n  17: patches/numeric.patch\n",
		"numeric path":            "packages: [.]\npatchedDependencies:\n  package@1.0.0: 17\n",
		"boolean path":            "packages: [.]\npatchedDependencies:\n  package@1.0.0: true\n",
		"null path":               "packages: [.]\npatchedDependencies:\n  package@1.0.0: null\n",
		"wrong shape":             "packages: [.]\npatchedDependencies: false\n",
		"multiple documents":      "packages: [.]\n---\npackages: [.]\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, true)
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
			if err := Configure(f.options()); err == nil {
				t.Fatal("malformed or ambiguous YAML configuration was accepted")
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
	f := newFixture(t, true)
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
	options.run = func(_ string, _ string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte(pnpmVersion), nil
		}
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

func TestConfigureRefusesDuplicateKitJSONKeysWithoutRewritingPackage(t *testing.T) {
	f := newFixture(t, true)
	manifestPath := filepath.Join(f.web, "package.json")
	manifest := []byte(`{"dependencies":{"@sveltejs/kit":"catalog:","@sveltejs/kit":"^3.0.0"}}`)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Configure(f.options()); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate Kit dependency key was not refused: %v", err)
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
		installStockKitBytes(t, f.web, "3.0.0", "@sveltejs/kit", []byte("altered source\n"))
		assertConfigureRefusesWithoutWrites(t, f, "unexpected SHA-256")
	})
	t.Run("incompatible exact Kit patch", func(t *testing.T) {
		f := newFixture(t, true)
		if err := setNativePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@3.0.0": "patches/other-kit.patch"}); err != nil {
			t.Fatal(err)
		}
		beforeConfig, err := os.ReadFile(filepath.Join(f.web, "pnpm-workspace.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		assertConfigureRefusesWithoutWrites(t, f, "conflicting @sveltejs/kit@3.0.0")
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
		if err := setNativePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@3.0.0": patchPath}); err != nil {
			t.Fatal(err)
		}
		assertConfigureRefusesWithoutWrites(t, f, "portable relative path")
	})
	t.Run("wildcard Kit patch", func(t *testing.T) {
		f := newFixture(t, true)
		if err := setNativePatchedDependencies(t, f, map[string]string{"@sveltejs/kit@^3": "patches/wildcard.patch"}); err != nil {
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
		if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"^3.0.0"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		parentConfig := filepath.Join(parent, "pnpm-workspace.yaml")
		before := []byte("packages:\n  - web\noverrides:\n  sentinel: 1.2.3\n")
		if err := os.WriteFile(parentConfig, before, 0o644); err != nil {
			t.Fatal(err)
		}
		f := fixture{web: web, pnpm: mustPNPM(t)}
		opts := f.options()
		opts.Workspace = parent
		if err := Configure(opts); err == nil || !strings.Contains(err.Error(), "refusing to modify parent pnpm workspace") {
			t.Fatalf("parent workspace was not refused: %v", err)
		}
		assertBytesUnchanged(t, parentConfig, before)
		assertBytesUnchanged(t, filepath.Join(web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"^3.0.0"}}`))
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
		if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(`{"dependencies":{"@sveltejs/kit":"^3.0.0"}}`), 0o644); err != nil {
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
		f := fixture{web: web, pnpm: mustPNPM(t)}
		if err := Configure(f.options()); err != nil {
			t.Fatal(err)
		}
		assertBytesUnchanged(t, parentConfig, parentBefore)
		selectedAfter, err := os.ReadFile(selectedConfig)
		if err != nil || !bytes.Contains(selectedAfter, []byte("patchedDependencies:")) {
			t.Fatalf("selected frontend workspace was not configured: %v\n%s", err, selectedAfter)
		}
	})
	t.Run("web symlink cannot hide parent workspace", func(t *testing.T) {
		parent := t.TempDir()
		web := filepath.Join(parent, "web")
		if err := os.MkdirAll(web, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := []byte(`{"dependencies":{"@sveltejs/kit":"^3.0.0"}}`)
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
		f := fixture{web: alias, pnpm: mustPNPM(t)}
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
	_, declaration, got, err := pinKit(input, "3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(input, []byte(`"@sveltejs/kit": "catalog:"`), []byte(`"@sveltejs/kit": "3.0.0"`), 1)
	if declaration != "catalog:" || !bytes.Equal(got, want) {
		t.Fatalf("pin changed unrelated package bytes or declaration = %q:\n%s", declaration, got)
	}
	if _, _, _, err := pinKit([]byte(`{"dependencies":{"@sveltejs/kit":"^3"},"devDependencies":{"@sveltejs/kit":"^3"}}`), "3.0.0"); err == nil {
		t.Fatal("duplicate Kit dependency sections were accepted")
	}
	for _, input := range []string{
		`{"dependencies":{"@sveltejs/kit":"catalog:","@sveltejs/kit":"^3.0.0"}}`,
		`{"dependencies":{"@sveltejs/kit":"catalog:"},"dependencies":{"@sveltejs/kit":"^3.0.0"}}`,
	} {
		if _, _, _, err := pinKit([]byte(input), "3.0.0"); err == nil {
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
	err = Configure(f.options())
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("Configure error = %v; want substring %q", err, wantError)
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

func installStockKit(t *testing.T, web, version, name string) {
	t.Helper()
	stockDir := filepath.Join("..", "..", "example", "web", "node_modules", "@sveltejs", "kit")
	stockManifest, err := os.ReadFile(filepath.Join(stockDir, "package.json"))
	if err != nil {
		t.Fatalf("installed pinned Kit source is required: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(stockManifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["name"] != kitPackageName || manifest["version"] != "3.0.0" {
		t.Fatalf("authoritative installed Kit identity = %v@%v", manifest["name"], manifest["version"])
	}
	queue, err := os.ReadFile(filepath.Join(stockDir, "src", "core", "postbuild", "queue.js"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, patch, err := adapter.KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	if digest(queue) == metadata.CorrectedSHA256 {
		queue = recoverStockQueue(t, metadata, patch, queue)
	}
	if digest(queue) != metadata.StockSHA256 {
		t.Fatalf("installed Kit queue source hash %s is neither packaged stock %s nor corrected %s", digest(queue), metadata.StockSHA256, metadata.CorrectedSHA256)
	}
	installStockKitBytes(t, web, version, name, queue)
}

func recoverStockQueue(t *testing.T, metadata adapter.KitQueueMetadata, patch, corrected []byte) []byte {
	t.Helper()
	root := t.TempDir()
	queuePath := filepath.Join(root, filepath.FromSlash(metadata.QueuePath))
	if err := os.MkdirAll(filepath.Dir(queuePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(queuePath, corrected, 0o644); err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(root, "queue.patch")
	if err := os.WriteFile(patchPath, patch, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "apply", "--reverse", patchPath)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recover reviewed stock queue in disposable copy: %v: %s", err, output)
	}
	stock, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	if digest(stock) != metadata.StockSHA256 {
		t.Fatalf("reverse-applied queue sha256 = %s; want embedded stock %s", digest(stock), metadata.StockSHA256)
	}
	return stock
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

func setNativePatchedDependencies(t *testing.T, f fixture, values map[string]string) error {
	t.Helper()
	jsonValue, err := json.Marshal(values)
	if err != nil {
		return err
	}
	cmd := exec.Command(f.pnpm, "config", "set", "patchedDependencies", string(jsonValue), "--location=project", "--json")
	cmd.Dir = f.web
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("native pnpm config set: %w: %s", err, output)
	}
	return nil
}

func getNativePatchedDependencies(t *testing.T, f fixture) map[string]string {
	t.Helper()
	cmd := exec.Command(f.pnpm, "config", "get", "patchedDependencies", "--location=project", "--json")
	cmd.Dir = f.web
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native pnpm config get: %v: %s", err, output)
	}
	var values map[string]string
	if strings.TrimSpace(string(output)) == "null" || strings.TrimSpace(string(output)) == "" {
		return map[string]string{}
	}
	if err := json.Unmarshal(output, &values); err != nil {
		t.Fatalf("parse native pnpm config JSON: %v: %s", err, output)
	}
	return values
}

func mustPNPM(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("pnpm")
	if err != nil {
		t.Fatal("native pnpm 12.9.1 is required")
	}
	return path
}

func assertBytesUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s changed or became unreadable: %v\n got %q\nwant %q", path, err, got, want)
	}
}
