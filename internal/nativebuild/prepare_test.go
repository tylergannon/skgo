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
		if name == "go" && args[0] == "build" {
			for i, arg := range args {
				if arg == "-o" {
					if err := os.WriteFile(args[i+1], []byte("fixture archive"), 0644); err != nil {
						return nil, err
					}
				}
			}
		}
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

func TestPrepareOnlyOmitsDebugBuild(t *testing.T) {
	o, commands, run := nativeFixture(t)
	o.PrepareOnly = true
	if err := build(o, run); err != nil {
		t.Fatal(err)
	}
	var zig, archive, project bool
	for _, command := range *commands {
		args := strings.Join(command.args, " ")
		if command.name == "xcodebuild" {
			t.Fatal("prepare compiled Debug")
		}
		zig = zig || strings.Contains(args, "zig build library -Dtarget=aarch64-macos -Doptimize=ReleaseFast -j4")
		archive = archive || strings.Contains(args, "-buildmode=c-archive")
		project = project || strings.Contains(args, "xcodegen generate")
	}
	if !zig || !archive || !project {
		t.Fatalf("missing native preparation: %v", *commands)
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
	found := false
	for _, command := range *commands {
		found = found || filepath.Base(command.name) == "vp" && reflect.DeepEqual(command.args, []string{"build"})
	}
	if !found {
		t.Fatal("frontend was not built")
	}
	last := (*commands)[len(*commands)-1]
	args := strings.Join(last.args, " ")
	if last.name != "xcodebuild" || !strings.Contains(args, "-configuration Debug") || strings.Count(args, "-jobs 4") != 1 {
		t.Fatalf("Debug build and worker limit: %+v", last)
	}
}
