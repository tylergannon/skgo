package nativebuild

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type nativeCommand struct {
	name string
	args []string
}

func nativeFixture(t *testing.T) (Options, *[]nativeCommand, func(string, []string, string, ...string) ([]byte, error)) {
	t.Helper()
	root, sdk := t.TempDir(), t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"web/build", "native"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{
		"go.mod":                       "module example.com/native-fixture\n\ngo 1.27\n",
		"skgo.json":                    `{"version":1,"name":"Fixture"}`,
		"native/skgo-native.json":      `{"version":1,"bundleId":"example.native.fixture"}`,
		"web/build/skgo.manifest.json": `{"fixture":"prepared frontend"}`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(sdk, "native/core"), 0755); err != nil {
		t.Fatal(err)
	}
	var commands []nativeCommand
	run := func(_ string, _ []string, name string, args ...string) ([]byte, error) {
		commands = append(commands, nativeCommand{name, append([]string(nil), args...)})
		if name == "go" && args[0] == "list" {
			return json.Marshal(map[string]string{"Dir": sdk})
		}
		if name == "xcrun" {
			return []byte("/fixture/toolchain"), nil
		}
		return nil, nil
	}
	return Options{Root: root, Out: io.Discard, Jobs: 4}, &commands, run
}

func TestPrepareOnlyKeepsNativeWorkAndOmitsFrontendAndDebugBuild(t *testing.T) {
	o, commands, run := nativeFixture(t)
	o.PrepareOnly, o.ReuseWebBuild = true, true
	if err := build(o, run); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, command := range *commands {
		names = append(names, filepath.Base(command.name))
	}
	if !reflect.DeepEqual(names, []string{"go", "go", "mise", "xcrun", "xcrun", "go", "mise"}) {
		t.Fatalf("preparation commands: %v", names)
	}
	wantZig := []string{"-C", filepath.Join(o.Root, "native/build/macos/.sdk/core"), "exec", "--", "zig", "build", "library", "-Dtarget=aarch64-macos", "-Doptimize=ReleaseFast", "-j4"}
	if got := (*commands)[2].args; !reflect.DeepEqual(got, wantZig) {
		t.Fatalf("Zig command: %q", got)
	}
	if got := strings.Join((*commands)[5].args, " "); !strings.Contains(got, "-buildmode=c-archive") {
		t.Fatalf("native Go archive not built: %s", got)
	}
	if got := strings.Join((*commands)[6].args, " "); !strings.Contains(got, "xcodegen generate") {
		t.Fatalf("preparation did not finish at project generation: %s", got)
	}
}

func TestNativeWorkerDefaultsAndInvalidLimit(t *testing.T) {
	o, commands, run := nativeFixture(t)
	o.Jobs = 0
	if err := build(o, run); err != nil {
		t.Fatal(err)
	}
	for _, command := range *commands {
		if command.name != "mise" && command.name != "xcodebuild" {
			continue
		}
		for _, arg := range command.args {
			if strings.HasPrefix(arg, "-j") {
				t.Fatalf("default build overrides tool worker count: %+v", command)
			}
		}
	}
	o.Jobs = -1
	*commands = nil
	if err := build(o, run); err == nil || len(*commands) != 0 {
		t.Fatalf("invalid worker count ran commands: err=%v commands=%v", err, *commands)
	}
}

func TestOrdinaryNativeBuildCompilesWebAndDebugWithWorkerLimit(t *testing.T) {
	o, commands, run := nativeFixture(t)
	if err := build(o, run); err != nil {
		t.Fatal(err)
	}
	if command := (*commands)[2]; filepath.Base(command.name) != "vp" || !reflect.DeepEqual(command.args, []string{"build"}) {
		t.Fatalf("frontend was not built: %+v", command)
	}
	last := (*commands)[len(*commands)-1]
	args := strings.Join(last.args, " ")
	if last.name != "xcodebuild" || !strings.Contains(args, "-configuration Debug") || strings.Count(args, "-jobs 4") != 1 {
		t.Fatalf("Debug build and worker limit: %+v", last)
	}
}

func TestReuseRequiresNonemptyFrontendManifest(t *testing.T) {
	for _, missing := range []bool{false, true} {
		o, commands, run := nativeFixture(t)
		o.ReuseWebBuild = true
		manifest := filepath.Join(o.Root, "web/build/skgo.manifest.json")
		var err error
		if missing {
			err = os.Remove(manifest)
		} else {
			err = os.WriteFile(manifest, nil, 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := build(o, run); err == nil || !strings.Contains(err.Error(), "requires a built frontend") {
			t.Fatalf("missing=%v: %v", missing, err)
		}
		for _, command := range *commands {
			if filepath.Base(command.name) == "vp" || command.name == "xcodebuild" {
				t.Fatalf("invalid prepared inputs reached compilation: %+v", command)
			}
		}
	}
}
