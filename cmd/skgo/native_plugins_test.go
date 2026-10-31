//go:build skgo_qualification

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/pluginbuild"
	"github.com/tylergannon/skgo/internal/plugininstall"
	"github.com/tylergannon/skgo/internal/pluginstore"
)

// Exercise the real loader, isolated builder, installer, generation publisher,
// and SDK commands together. A local versioned host is not a published-host
// certification. No Swift, Zig or .NET compiler is needed for this boundary.
func TestNativeShellPluginsCoexist(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host := provisionalCapabilityHostVersion(t, repo, root, "v0.41.1-native-qualification")
	if err := os.MkdirAll(filepath.Join(root, "manual"), 0755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "application")
	write := func(name, text string) {
		t.Helper()
		full := filepath.Join(project, name)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/native-consumer\n\ngo 1.27.3\nrequire github.com/tylergannon/skgo v0.41.1-native-qualification\n")
	write("skgo.json", `{"version":1,"name":"BothShells"}`)
	write("internal/app/locals.go", "package app\ntype Locals struct{}\n")
	if err := os.MkdirAll(filepath.Join(project, "web/src"), 0755); err != nil {
		t.Fatal(err)
	}

	write("native/Application.swift", "// authored Swift\n")
	write("windows/Application.cs", "// authored C#\n")
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), host, args...)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "HOME="+root, "SKGO_PLUGIN_DIRS="+filepath.Join(root, "manual"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	require := func(args ...string) string {
		t.Helper()
		out, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	generate := []string{"generate", "--web", filepath.Join(project, "web"), "--out", filepath.Join(project, "bindings"), "--locals-package", "example.test/native-consumer/internal/app"}
	// Start with the same generated event/locals boundary an empty app gets.
	require(generate...)
	write("web/src/routes/value.remote.go", `package value
import ("context"; "github.com/tylergannon/skgo"; "example.test/native-consumer/bindings/params")
func readValue(context.Context) (string,error) { return "initial",nil }
func writeValue(_ context.Context,_ params.RequestEvent,next string) (string,error) { return next,nil }
var _ = skgo.Query(readValue)
var _ = skgo.Command(writeValue)
`)
	tidy := exec.CommandContext(t.Context(), "go", "mod", "tidy")
	tidy.Dir = project
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("resolve consumer dependencies: %v\n%s", err, out)
	}
	generate = append(generate, "--plugins")
	var artifacts []string
	for _, id := range []string{"native-shell", "windows-shell"} {
		artifact := filepath.Join(root, fmt.Sprintf("%s-v0.1.0-skgo-v0.41.1-native-qualification-%s-%s.so", id, runtime.GOOS, runtime.GOARCH))
		var log bytes.Buffer
		if err := pluginbuild.Build(t.Context(), pluginbuild.Options{Host: host, Source: filepath.Join(repo, "plugins", id), Output: artifact, PluginVersion: "v0.1.0", VersionSymbol: "main.SkgoVersion", Log: &log}); err != nil {
			t.Fatalf("build %s: %v\n%s", id, err, &log)
		}
		artifacts = append(artifacts, artifact)
		digest, err := pluginstore.Digest(artifact)
		if err != nil {
			t.Fatal(err)
		}
		require("plugin", "install", "--prebuilt", artifact, "--sha256", digest, "--select", "src/routes/value.remote.ts#readValue", "--select", "src/routes/value.remote.ts#writeValue")
	}
	// Both load orders must qualify through the host's actual peer probe.
	for _, order := range [][]string{artifacts, {artifacts[1], artifacts[0]}} {
		for _, candidate := range order {
			encoded, _ := json.Marshal(order)
			report := filepath.Join(root, "probe.json")
			cmd := exec.CommandContext(t.Context(), host, "_plugin-probe", candidate, report)
			cmd.Env = append(os.Environ(), pluginstore.ProbeFiles+"="+string(encoded))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("peer probe: %v\n%s", err, out)
			}
			data, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var exports plugininstall.Exports
			if err := json.Unmarshal(data, &exports); err != nil {
				t.Fatal(err)
			}
			if len(exports.Diagnostics) != 0 || exports.ID == "" {
				t.Fatalf("probe=%s", data)
			}
		}
	}
	require(generate...)
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(project, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	swift, csharp := read("native/Generated.swift"), read("windows/Generated.cs")
	if !bytes.Contains(swift, []byte("public func readValue(")) || !bytes.Contains(csharp, []byte("readValueAsync(")) {
		t.Fatal("missing generated typed calls")
	}
	for range 2 {
		require(generate...)
		require("plugin", "native-shell", "sdk")
		require("plugin", "windows-shell", "sdk")
	}
	if !bytes.Equal(swift, read("native/Generated.swift")) || !bytes.Equal(csharp, read("windows/Generated.cs")) {
		t.Fatal("repeat generation changed output")
	}
	for _, part := range []string{"src/abi.zig", "src/cache.zig", "include/skgo.h", "build.zig"} {
		if !bytes.Equal(read("native/SDK/core/"+part), read("windows/SDK/core/"+part)) {
			t.Fatalf("different shared source %s", part)
		}
	}
	read("native/SDK/swift/Sources/CSKGo/include/skgo.h")
	read("windows/SDK/csharp/SKGoNative/RemoteClient.cs")
	for _, name := range []string{"windows/SDK/swift", "native/SDK/csharp"} {
		if _, err := os.Stat(filepath.Join(project, name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected %s: %v", name, err)
		}
	}
	for _, attempt := range [][]string{{"plugin", "native-shell", "sdk", "--out", "windows/SDK"}, {"plugin", "windows-shell", "sdk", "--out", "native/SDK"}, {"plugin", "windows-shell", "sdk", "--out", "native"}, {"plugin", "native-shell", "sdk", "--out", "windows"}} {
		if out, err := run(attempt...); err == nil || !strings.Contains(out, "not disposable") {
			t.Fatalf("SDK protection %v: %v %s", attempt, err, out)
		}
	}
	if string(read("native/Application.swift")) != "// authored Swift\n" || string(read("windows/Application.cs")) != "// authored C#\n" {
		t.Fatal("authored application source changed")
	}
	for _, edited := range []string{"native/Generated.swift", "windows/Generated.cs"} {
		original := read(edited)
		write(edited, "// authored edit\n")
		if out, err := run(generate...); err == nil || !strings.Contains(out, "authored or edited") {
			t.Fatalf("edited output protection: %v %s", err, out)
		}
		if string(read(edited)) != "// authored edit\n" {
			t.Fatal("overwrote authored edit")
		}
		write(edited, string(original))
	}
	if !bytes.Equal(swift, read("native/Generated.swift")) || !bytes.Equal(csharp, read("windows/Generated.cs")) {
		t.Fatal("one generator removed peer output")
	}
	t.Log("Both real plugins loaded in either order; same-project repeated generation and SDK export preserve peer outputs and authored files; each bundles the same native core with its own language SDK")
}
