package newapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreationUsesQualifiedVPEnvironment(t *testing.T) {
	for _, tc := range []struct{ vp, pnpm, wantError string }{
		{"v0.3.3", "12.9.1", "requires qualified VitePlus 1.0.0"},
		{"v1.0.0-beta.1", "12.9.1", "requires qualified VitePlus 1.0.0"},
		{"v1.0.1", "12.9.1", "requires qualified VitePlus 1.0.0"},
		{"v1.0.0", "12.10.1", "vp environment requires pnpm 12.9.1"},
		{"v1.0.0", "12.9.1", ""},
	} {
		t.Run(tc.vp+"-"+tc.pnpm, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "qualified")
			managed := filepath.Join(root, "managed")
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(managed, 0755); err != nil {
				t.Fatal(err)
			}
			vp := filepath.Join(bin, "vp")
			script := `#!/bin/sh
if [ "$1" = --version ]; then printf 'vp %s\nLocal vite-plus:\n' "$FIXTURE_VP_VERSION"; exit 0; fi
if [ "$1" = create ]; then
 if [ -n "$FIXTURE_BOOTSTRAP_RECEIPT" ]; then cat package.json > "$FIXTURE_BOOTSTRAP_RECEIPT" || exit 48; fi
 printf 'created with %s' "$(pnpm --version)"
 exit "${FIXTURE_CREATE_EXIT:-0}"
fi
[ "$1 $2 $3 $4" = 'env exec --package-manager pnpm@12.9.1' ] || exit 41
[ "$VP_PACKAGE_MANAGER" = pnpm@12.9.1 ] || exit 42
[ "$VP_PNPM_VERSION" = 12.9.1 ] || exit 43
shift 4
export PATH="$FIXTURE_BIN:$PATH"
if [ "$1 $2" = 'which pnpm' ]; then
 printf 'installing companion (stderr only)\n' >&2
 printf '%s/pnpm\n' "$FIXTURE_BIN"
 exit 0
fi
exec "$@"
`
			if err := os.WriteFile(vp, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(managed, "pnpm"), []byte("#!/bin/sh\nprintf '%s\\n' \"$FIXTURE_PNPM_VERSION\"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "pnpm"), []byte("#!/bin/sh\nprintf '12.10.1\\n'\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FIXTURE_VP_VERSION", tc.vp)
			t.Setenv("FIXTURE_PNPM_VERSION", tc.pnpm)
			t.Setenv("FIXTURE_BIN", managed)
			t.Setenv("VP_PACKAGE_MANAGER", "pnpm@99.0.0")
			t.Setenv("VP_PNPM_VERSION", "99.0.0")
			// Resolve the selected executable before running from another directory.
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			var out bytes.Buffer
			run, pnpm, err := qualifiedRunner("vp", root, &out, io.Discard)
			// Exercise ordinary Create's wiring too. Stop at the actual creator
			// subprocess so this focused bootstrap test needs no frontend install.
			var creation bytes.Buffer
			receipt := filepath.Join(root, "bootstrap-receipt.json")
			t.Setenv("FIXTURE_BOOTSTRAP_RECEIPT", receipt)
			t.Setenv("FIXTURE_CREATE_EXIT", "47")
			_, createErr := Create(Options{
				Dir: filepath.Join(root, "application"), Module: "example.com/bootstrap", App: "bootstrap",
				SkgoVersion: "v0.28.0", SVAddonSpec: "@skgo/sv@0.28.0", AdapterSpec: "@skgo/sveltekit-adapter@0.28.0",
				RegistryURL: "https://registry.invalid", Client: &http.Client{Transport: versionResponseTransport{}},
				Stdout: &creation, Stderr: io.Discard,
			})
			t.Setenv("FIXTURE_CREATE_EXIT", "0")
			t.Setenv("FIXTURE_BOOTSTRAP_RECEIPT", "")
			if tc.wantError == "" {
				if createErr == nil || !strings.Contains(createErr.Error(), "VitePlus project creation failed") || creation.String() != "created with 12.9.1" {
					t.Fatalf("ordinary creation did not reach the qualified creator: %v, %q", createErr, &creation)
				}
				assertBootstrapSelection(t, receipt)
			} else if createErr == nil || !strings.Contains(createErr.Error(), tc.wantError) || creation.Len() != 0 {
				t.Fatalf("ordinary creation bypassed qualification: %v, %q", createErr, &creation)
			}
			if _, err := os.Stat(filepath.Join(root, "application", "package.json")); !os.IsNotExist(err) {
				t.Fatalf("creation retained root JavaScript metadata: %v", err)
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v; want %s", err, tc.wantError)
				}
				if run != nil || pnpm != "" || out.Len() != 0 {
					t.Fatal("unqualified environment became usable")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if pnpm != filepath.Join(managed, "pnpm") {
				t.Fatalf("Kit patch executable = %q", pnpm)
			}
			// Every child, including project-local vp and Go generation, must inherit
			// the same companion selection without mutating the caller's environment.
			env := os.Environ()
			before := strings.Join(env, "\n")
			if err := run(command{Dir: root, Name: "vp", Args: []string{"create"}, Env: env}); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != "created with 12.9.1" {
				t.Fatalf("creation = %q", got)
			}
			out.Reset()
			if err := run(command{Dir: root, Name: "pnpm", Args: []string{"--version"}, Env: env}); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(out.String()); got != "12.9.1" {
				t.Fatalf("child companion = %q", got)
			}
			if strings.Join(env, "\n") != before || os.Getenv("VP_PNPM_VERSION") != "99.0.0" {
				t.Fatal("caller environment changed")
			}
		})
	}
}

func TestCreationBootstrapMetadataIsTemporary(t *testing.T) {
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		var called bool
		failure := errors.New("creator failed")
		err := createFrontend(func(c command) error {
			called = true
			assertBootstrapSelection(t, filepath.Join(c.Dir, "package.json"))
			if fail {
				return failure
			}
			return nil
		}, command{Dir: dir})
		if !called || (fail && !errors.Is(err, failure)) || (!fail && err != nil) {
			t.Fatalf("creation result: called=%v, failure=%v, err=%v", called, fail, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "package.json")); !os.IsNotExist(err) {
			t.Fatalf("temporary bootstrap metadata survived creation (failure=%v): %v", fail, err)
		}
	}
}

func assertBootstrapSelection(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Private        bool   `json:"private"`
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || !pkg.Private || pkg.PackageManager != "pnpm@12.9.1" {
		t.Fatalf("upstream creation bootstrap selection = %s, %v", data, err)
	}
}
