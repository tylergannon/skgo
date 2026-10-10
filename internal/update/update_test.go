package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/toolchain"
)

func TestNewExecutableOwnsCompletionAndCacheIsNotInstalledOver(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "project-tool"}[cached], func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(root, "bin")
			cache := filepath.Join(root, "cache")
			os.MkdirAll(bin, 0755)
			os.MkdirAll(cache, 0755)
			old := filepath.Join(bin, "skgo")
			if cached {
				old = filepath.Join(cache, "tool-executable")
			}
			os.WriteFile(old, []byte("original host"), 0755)
			var calls []command
			var out bytes.Buffer
			run := func(ctx context.Context, c command) ([]byte, error) {
				calls = append(calls, c)
				switch {
				case reflect.DeepEqual(c.Args, []string{"env", "-json", "GOBIN", "GOPATH", "GOCACHE"}):
					return json.Marshal(map[string]string{"GOBIN": bin, "GOCACHE": cache})
				case reflect.DeepEqual(c.Args, []string{"list", "-m", "-json", buildinfo.Module + "@latest"}):
					return []byte(`{"Version":"v9.1.0"}`), nil
				case len(c.Args) > 0 && c.Args[0] == "install":
					if !reflect.DeepEqual(c.Env, []string{"GOWORK=off", "GOBIN=" + bin}) {
						t.Fatalf("install env %v", c.Env)
					}
					return nil, nil
				case reflect.DeepEqual(c.Args, []string{"buildinfo", "--json"}):
					return []byte(`{"skgoVersion":"v9.1.0"}`), nil
				case len(c.Args) > 0 && c.Args[0] == "update":
					if c.Name != filepath.Join(bin, "skgo") || !reflect.DeepEqual(c.Args, []string{"update", "--complete-version", "v9.1.0", "--vp", "/chosen/vp"}) {
						t.Fatalf("completion: %+v", c)
					}
					return nil, errors.New("controlled target failed vp alignment")
				}
				t.Fatalf("old executable tried extra work: %+v", c)
				return nil, nil
			}
			err = Run(context.Background(), Options{Out: &out, VP: "/chosen/vp", run: run, info: func() (buildinfo.Info, error) { return buildinfo.Info{Executable: old, SkgoVersion: "v9.0.0"}, nil }})
			if err == nil || !strings.Contains(err.Error(), "installed, but toolchain/project update is incomplete") {
				t.Fatalf("partial outcome %v", err)
			}
			if len(calls) != 5 {
				t.Fatalf("calls=%v", calls)
			}
			if b, _ := os.ReadFile(old); string(b) != "original host" {
				t.Fatal("overwrote original/cache host in test")
			}
		})
	}
}
func TestCompletionAlignsExactVitePlusIncludingDowngrade(t *testing.T) {
	var calls []command
	versions := 0
	err := Run(context.Background(), Options{VP: "/global/vp", CompleteVersion: "v9.1.0", info: func() (buildinfo.Info, error) { return buildinfo.Info{SkgoVersion: "v9.1.0"}, nil }, run: func(ctx context.Context, c command) ([]byte, error) {
		calls = append(calls, c)
		if reflect.DeepEqual(c.Args, []string{"--version"}) {
			versions++
			if versions == 1 {
				return []byte("vp v9.0.0\n"), nil
			}
			return []byte("vp v" + toolchain.VitePlus + "\n"), nil
		}
		if !reflect.DeepEqual(c.Args, []string{"upgrade", toolchain.VitePlus}) {
			t.Fatalf("unexpected step %+v", c)
		}
		return nil, nil
	}})
	if err != nil || len(calls) != 3 {
		t.Fatalf("completion %v calls=%v", err, calls)
	}
}
func TestCompletionRefusesWrongBinaryBeforeMutation(t *testing.T) {
	err := Run(context.Background(), Options{CompleteVersion: "v2.0.0", info: func() (buildinfo.Info, error) { return buildinfo.Info{SkgoVersion: "v1.0.0"}, nil }, run: func(context.Context, command) ([]byte, error) {
		t.Fatal("mutation before version check")
		return nil, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "completion requires") {
		t.Fatal(err)
	}
}
func TestFrontendMigrationPreservesSourceAndPinsCompanion(t *testing.T) {
	source, stage := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(source, "package.json"), []byte(`{"name":"app","scripts":{"custom":"keep"},"devDependencies":{"vite-plus":"0.9.0","@skgo/sveltekit-adapter":"0.1.0"},"devEngines":{"runtime":{"name":"node","version":"24.0.0"}}}`), 0644)
	os.Mkdir(filepath.Join(source, "src"), 0755)
	os.WriteFile(filepath.Join(source, "src", "page.svelte"), []byte("<h1>My edited application</h1>"), 0644)
	if err := copyFrontend(source, stage); err != nil {
		t.Fatal(err)
	}
	if err := pinDependencies(stage, "0.28.0", "0.27.0", "3.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := preservedFrontend(source, stage); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(stage, "package.json"))
	var p map[string]any
	json.Unmarshal(b, &p)
	if p["packageManager"] != "pnpm@"+toolchain.PNPM || p["devDependencies"].(map[string]any)["vite-plus"] != toolchain.VitePlus || p["devDependencies"].(map[string]any)["@skgo/sveltekit-adapter"] != "0.28.0" || p["scripts"].(map[string]any)["custom"] != "keep" {
		t.Fatalf("pins/source %s", b)
	}
	if p["devEngines"].(map[string]any)["runtime"].(map[string]any)["version"] != "24.0.0" {
		t.Fatal("rewrote runtime preference")
	}
	os.WriteFile(filepath.Join(stage, "src", "page.svelte"), []byte("overwritten"), 0644)
	if err := preservedFrontend(source, stage); err == nil || !strings.Contains(err.Error(), "src/page.svelte") {
		t.Fatalf("lost source conflict %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(source, "src", "page.svelte")); string(b) != "<h1>My edited application</h1>" {
		t.Fatal("mutated authored source")
	}
}
