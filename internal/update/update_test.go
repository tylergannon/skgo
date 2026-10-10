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

func TestMigrationRejectsAuthoredConfigurationChanges(t *testing.T) {
	for _, tc := range []struct{ name, file, before, after string }{
		{"script", "package.json", `{"scripts":{"build":"my build"}}`, `{"scripts":{"build":"vp build"}}`},
		{"runtime", "package.json", `{"devEngines":{"runtime":{"version":"24"}}}`, `{"devEngines":{"runtime":{"version":"26"}}}`},
		{"application dependency", "package.json", `{"dependencies":{"my-library":"1.0.0"}}`, `{"dependencies":{"my-library":"2.0.0"}}`},
		{"override", "pnpm-workspace.yaml", "overrides:\n  my-library: 1.0.0\n", "overrides:\n  my-library: 2.0.0\n"},
		{"unrelated patch", "pnpm-workspace.yaml", "patchedDependencies:\n  my-library@1.0.0: patches/mine.patch\n", "patchedDependencies: {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, stage := t.TempDir(), t.TempDir()
			os.WriteFile(filepath.Join(source, tc.file), []byte(tc.before), 0644)
			os.WriteFile(filepath.Join(stage, tc.file), []byte(tc.after), 0644)
			if err := preservedFrontend(source, stage); err == nil || !strings.Contains(err.Error(), "authored configuration") {
				t.Fatalf("configuration loss accepted: %v", err)
			}
		})
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"original"}`), 0644)
	original, err := frontendFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"edited while staging"}`), 0644)
	if err := unchangedFrontend(root, original); err == nil {
		t.Fatal("concurrent edit accepted")
	}
}

func TestTemporaryGoToolDoesNotBecomeInstallationDirectory(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/cache/aa/abc-d", true},
		{"/private/var/folders/example/T/go-build1234/b001/exe/skgo", true},
		{"/tmp/my-install/skgo", false},
		{"/tmp/go-build-user/bin/skgo", false},
		{"/usr/local/bin/skgo", false},
	} {
		if got := goToolExecutable("/cache", tc.path); got != tc.want {
			t.Errorf("%s: %v", tc.path, got)
		}
	}
}

func TestProjectToolUsesRetainedExecutableAfterFirstCompilation(t *testing.T) {
	root := t.TempDir()
	cached := filepath.Join(root, "hash-d")
	os.WriteFile(cached, []byte("retained tool"), 0755)
	calls := 0
	o := Options{Out: &bytes.Buffer{}, run: func(ctx context.Context, c command) ([]byte, error) {
		calls++
		if c.Dir != root || c.Name != "go" || !reflect.DeepEqual(c.Args, []string{"tool", "skgo", "buildinfo", "--json"}) {
			t.Fatalf("wrong invocation %+v", c)
		}
		path := cached
		if calls == 1 {
			path = filepath.Join(root, "go-build123/b001/exe/skgo")
		}
		return json.Marshal(buildinfo.Info{SkgoVersion: "v9.1.0", Executable: path})
	}}
	got, err := projectTool(context.Background(), o, root, "v9.1.0")
	if err != nil || got != cached || calls != 2 {
		t.Fatalf("host=%s calls=%d err=%v", got, calls, err)
	}
}

func TestUpdateConsumesNativeConfigurationAndReportsNativeFailure(t *testing.T) {
	root := t.TempDir()
	calls := 0
	run := func(dir, name string, args ...string) error {
		calls++
		if dir != root || name != "skgo" || !reflect.DeepEqual(args, []string{"native", "build", "--root", root, "--platform", "simulator", "--preset", "synthetic"}) {
			t.Fatalf("native command %s %s %v", dir, name, args)
		}
		return errors.New("Swift compilation failed")
	}
	if err := completeNative(root, "simulator", "synthetic", run); err != nil || calls != 0 {
		t.Fatalf("non-native app: %v", err)
	}
	os.Mkdir(filepath.Join(root, "native"), 0755)
	os.WriteFile(filepath.Join(root, "native/skgo-native.json"), []byte(`{"version":1}`), 0644)
	err := completeNative(root, "simulator", "synthetic", run)
	if err == nil || !strings.Contains(err.Error(), "native generation/build for simulator remains incomplete") || calls != 1 {
		t.Fatalf("false completion: %v, calls %d", err, calls)
	}
}
