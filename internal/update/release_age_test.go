package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/tylergannon/skgo/internal/buildinfo"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAdapterAgeExceptionPreservesPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		accept              bool
	}{
		{"new exact adapter", "", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.2']\n", true},
		{"pnpm merges old adapter", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.22.1', 'authored@1.2.3']\n", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.22.1 || 0.28.2', 'authored@1.2.3']\n", true},
		{"wrong adapter version", "", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.3']\n", false},
		{"bare adapter", "", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter']\n", false},
		{"wildcard", "", "minimumReleaseAgeExclude: ['@skgo/*']\n", false},
		{"range", "", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@^0.28.2']\n", false},
		{"unrelated addition", "", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.2', 'authored@1.2.3']\n", false},
		{"removed exemption", "minimumReleaseAgeExclude: ['authored@1.2.3']\n", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.2']\n", false},
		{"removed old adapter", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.22.1']\n", "minimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.2']\n", false},
		{"changed age", "minimumReleaseAge: 1440\n", "minimumReleaseAge: 0\nminimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.2']\n", false},
		{"disabled strict", "minimumReleaseAgeStrict: true\n", "minimumReleaseAgeStrict: false\nminimumReleaseAgeExclude: ['@skgo/sveltekit-adapter@0.28.2']\n", false},
		{"nonstring exclusion", "", "minimumReleaseAgeExclude: [17]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage := t.TempDir()
			before := map[string][]byte{"pnpm-workspace.yaml": []byte(tc.before)}
			writeAlignmentFile(t, stage, "pnpm-workspace.yaml", tc.after)
			err := preservedFiles(before, stage, "0.28.2")
			if (err == nil) != tc.accept {
				t.Fatalf("guard = %v, accept=%v", err, tc.accept)
			}
			got, err := os.ReadFile(filepath.Join(stage, "pnpm-workspace.yaml"))
			if err != nil || string(got) != tc.after {
				t.Fatalf("guard mutated workspace: %q %v", got, err)
			}
		})
	}
}

// A controlled registry makes a newly published adapter deterministic, even
// after the real release ages out. This executes the qualified package manager
// that produced #297's failure, including its strict-policy refusal.
func TestQualifiedPNPMAdapterAgeException(t *testing.T) {
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		t.Fatal(err)
	}
	version, err := exec.Command(pnpm, "--version").Output()
	if err != nil || strings.TrimSpace(string(version)) != "12.9.1" {
		t.Fatalf("requires qualified pnpm12.9.1: %s %v", version, err)
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	pkg := []byte(`{"name":"@skgo/sveltekit-adapter","version":"0.28.2"}`)
	if err := tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0644, Size: int64(len(pkg))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(pkg); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512(archive.Bytes())
	var registry *httptest.Server
	registry = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/adapter.tgz" {
			w.Write(archive.Bytes())
			return
		}
		if r.URL.Path != "/@skgo/sveltekit-adapter" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"name": "@skgo/sveltekit-adapter", "dist-tags": map[string]string{"latest": "0.28.2"}, "time": map[string]string{"0.28.2": time.Now().UTC().Format(time.RFC3339)}, "versions": map[string]any{"0.28.2": map[string]any{"name": "@skgo/sveltekit-adapter", "version": "0.28.2", "dist": map[string]string{"tarball": registry.URL + "/adapter.tgz", "integrity": "sha512-" + base64.StdEncoding.EncodeToString(hash[:])}}}})
	}))
	defer registry.Close()
	for _, strict := range []bool{false, true} {
		name := "default non-strict"
		if strict {
			name = "authored strict"
		}
		t.Run(name, func(t *testing.T) {
			source, stage := t.TempDir(), t.TempDir()
			writeAlignmentFile(t, source, "package.json", `{"private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"@skgo/sveltekit-adapter":"0.22.1"}}`)
			policy := "allowBuilds:\n  esbuild: true\n"
			if strict {
				policy += "minimumReleaseAge: 1440\n"
			}
			writeAlignmentFile(t, source, "pnpm-workspace.yaml", policy)
			writeAlignmentFile(t, source, "receipt.txt", "authored receipt stays\n")
			before, err := frontendFiles(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := copyFrontend(source, stage); err != nil {
				t.Fatal(err)
			}
			writeAlignmentFile(t, stage, "package.json", `{"private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"@skgo/sveltekit-adapter":"0.28.2"}}`)
			cmd := exec.Command(pnpm, "install", "--no-frozen-lockfile", "--ignore-scripts", "--registry", registry.URL)
			cmd.Dir = stage
			cmd.Env = append(os.Environ(), "CI=1")
			output, installErr := cmd.CombinedOutput()
			t.Logf("pnpm install: %s", output)
			after, err := os.ReadFile(filepath.Join(stage, "pnpm-workspace.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("before workspace:\n%s\nafter workspace:\n%s", policy, after)
			if strict {
				if installErr == nil || !strings.Contains(string(output), "ERR_PNPM_NO_MATURE_MATCHING_VERSION") {
					t.Fatalf("strict policy did not reject fresh adapter: %v\n%s", installErr, output)
				}
				if string(after) != policy {
					t.Fatalf("strict install changed authored policy: %s", after)
				}
			} else {
				if installErr != nil {
					t.Fatalf("install: %v\n%s", installErr, output)
				}
				if !strings.Contains(string(after), "@skgo/sveltekit-adapter@0.28.2") {
					t.Fatalf("real pnpm did not persist exact exception: %s", after)
				}
				if err := preservedFiles(before, stage); err == nil {
					t.Fatal("unqualified guard accepted policy change")
				}
				if err := preservedFiles(before, stage, "0.28.2"); err != nil {
					t.Fatal(err)
				}
			}
			if err := unchangedFrontend(source, before); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Exercise the public dispatch and new host's completion with controlled child
// commands. The real pnpm behavior is covered above; this checks that the
// selected exception reaches the production guard before any project mutation.
func TestPublicUpdatePassesSelectedAdapterToGuard(t *testing.T) {
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		t.Fatal(err)
	}
	kitQueue, err := os.ReadFile("../../example/web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, exception := range []string{"@skgo/sveltekit-adapter@0.28.2", "unrelated@1.2.3"} {
		t.Run(exception, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(t.TempDir(), "bin")
			host := filepath.Join(bin, "skgo")
			writeAlignmentFile(t, root, "go.mod", "module example.com/age-update\n\ngo 1.27.1\nrequire github.com/tylergannon/skgo v0.27.0\n")
			writeAlignmentFile(t, root, "web/package.json", `{"private":true,"devDependencies":{"vite-plus":"1.0.0","@skgo/sveltekit-adapter":"0.22.1"},"scripts":{"receipt":"echo keep"}}`)
			writeAlignmentFile(t, root, "web/pnpm-workspace.yaml", "allowBuilds:\n  esbuild: true\n")
			before, err := frontendFiles(root)
			if err != nil {
				t.Fatal(err)
			}
			stop := errors.New("stopped before Go project mutation")
			reachedMutation, dispatched := false, false
			var opts Options
			opts = Options{Root: root, VP: "/qualified/vp", packages: func(string) (string, string, error) { return "0.28.2", "0.17.1", nil }, info: func() (buildinfo.Info, error) { return buildinfo.Info{Executable: host, SkgoVersion: "v0.28.2"}, nil }}
			opts.run = func(ctx context.Context, c command) ([]byte, error) {
				switch {
				case c.Name == "go" && slices.Equal(c.Args, []string{"env", "-json", "GOBIN", "GOPATH", "GOCACHE"}):
					return json.Marshal(map[string]string{"GOBIN": bin, "GOCACHE": filepath.Join(root, "cache")})
				case c.Name == "go" && slices.Equal(c.Args, []string{"list", "-m", "-json", "github.com/tylergannon/skgo@latest"}):
					return []byte(`{"Version":"v0.28.2"}`), nil
				case c.Name == "go" && slices.Equal(c.Args, []string{"install", "github.com/tylergannon/skgo/cmd/skgo@v0.28.2"}):
					return nil, nil
				case c.Name == host && slices.Equal(c.Args, []string{"buildinfo", "--json"}):
					return []byte(`{"skgoVersion":"v0.28.2"}`), nil
				case c.Name == host && len(c.Args) > 0 && c.Args[0] == "update":
					if !slices.Equal(c.Args, []string{"update", "--complete-version", "v0.28.2", "--root", root, "--vp", "/qualified/vp"}) {
						t.Fatalf("wrong reexec: %v", c.Args)
					}
					dispatched = true
					next := opts
					next.CompleteVersion = "v0.28.2"
					return nil, Run(ctx, next)
				case c.Name == "/qualified/vp" && slices.Equal(c.Args, []string{"--version"}):
					return []byte("vp v1.0.0\n"), nil
				case c.Name == "/qualified/vp" && slices.Equal(c.Args, []string{"toolchain", "--global", "--json", "vite", "vitest"}):
					return []byte(qualifiedVPGraph), nil
				case c.Name == "/qualified/vp" && slices.Equal(c.Args, []string{"env", "exec", "--package-manager", "pnpm@12.9.1", "which", "pnpm"}):
					return []byte(pnpm), nil
				case c.Name == "/qualified/vp" && slices.Equal(c.Args, []string{"env", "exec", "--package-manager", "pnpm@12.9.1", pnpm, "--version"}):
					return []byte("12.9.1\n"), nil
				case c.Name == "/qualified/vp" && slices.Equal(c.Args, []string{"env", "exec", "--package-manager", "pnpm@12.9.1", "/qualified/vp", "install", "--no-frozen-lockfile"}):
					path := filepath.Join(c.Dir, "pnpm-workspace.yaml")
					b, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					writeAlignmentFile(t, c.Dir, "pnpm-workspace.yaml", string(b)+"minimumReleaseAgeExclude: ['"+exception+"']\n")
					writeAlignmentFile(t, c.Dir, "node_modules/@sveltejs/kit/package.json", `{"name":"@sveltejs/kit","version":"3.0.0"}`)
					writeAlignmentFile(t, c.Dir, "node_modules/@sveltejs/kit/src/core/postbuild/queue.js", string(kitQueue))
					return nil, nil
				case c.Name == "go" && slices.Equal(c.Args, []string{"get", "github.com/tylergannon/skgo@v0.28.2"}):
					reachedMutation = true
					return nil, stop
				default:
					t.Fatalf("unexpected command: %+v", c)
					return nil, nil
				}
			}
			err = Run(context.Background(), opts)
			if !dispatched {
				t.Fatal("public update did not dispatch completion")
			}
			if exception == "@skgo/sveltekit-adapter@0.28.2" {
				if !reachedMutation || !errors.Is(err, stop) {
					t.Fatalf("selected adapter did not pass production guard: %v", err)
				}
			} else if reachedMutation || err == nil || !strings.Contains(err.Error(), "minimumReleaseAgeExclude") {
				t.Fatalf("unrelated exception passed production guard: %v", err)
			}
			if err := unchangedFrontend(root, before); err != nil {
				t.Fatal(err)
			}
		})
	}
}
