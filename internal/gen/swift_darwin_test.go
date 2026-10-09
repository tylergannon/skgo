//go:build darwin

package gen

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratedSwiftCallsTheGeneratedGoHandlers(t *testing.T) {
	root, cfg := generatedNativeFixture(t)
	repo, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	run := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	core := filepath.Join(repo, "native", "core")
	run(core, "mise", "exec", "--", "zig", "build", "install")
	scratch := t.TempDir()
	run(repo, "swift", "build", "--package-path", filepath.Join(repo, "native", "swift"), "--scratch-path", scratch)
	binPath := exec.CommandContext(ctx, "swift", "build", "--package-path", filepath.Join(repo, "native", "swift"), "--scratch-path", scratch, "--show-bin-path")
	binPath.Dir = repo
	out, err := binPath.CombinedOutput()
	if err != nil {
		t.Fatalf("swift build --show-bin-path: %v\n%s", err, out)
	}
	products := strings.TrimSpace(string(out))
	probe := filepath.Join(root, "app", "native", "Probe.swift")
	writeSharedFixture(t, filepath.Join(root, "app"), "native/Probe.swift", nativeSwiftConsumer)
	args := []string{"-swift-version", "6", "-parse-as-library", cfg.SwiftOut, probe, "-I", products, "-I", filepath.Join(products, "Modules"), "-I", filepath.Join(scratch, "debug", "CSKGo.build"), "-I", filepath.Join(repo, "native", "swift", "Sources", "CSKGo", "include"), "-L", filepath.Join(core, "zig-out", "lib"), "-lskgo_native_core"}
	// Swift's newer default build system emits one object per target in its
	// products directory. The native build system emits per-source objects.
	if _, err := os.Stat(filepath.Join(products, "SKGoNative.o")); err == nil {
		args = append(args, "-Xcc", "-fmodule-map-file="+filepath.Join(scratch, "out", "Intermediates.noindex", "GeneratedModuleMaps", "CSKGo.modulemap"))
		args = append(args, filepath.Join(products, "SKGoNative.o"), filepath.Join(products, "CSKGo.o"))
	} else {
		for _, pattern := range []string{"SKGoNative.build/*.swift.o", "CSKGo.build/*.o"} {
			objects, err := filepath.Glob(filepath.Join(scratch, "debug", pattern))
			if err != nil || len(objects) == 0 {
				t.Fatalf("Swift objects %s: %v", pattern, err)
			}
			args = append(args, objects...)
		}
	}
	cli := filepath.Join(scratch, "native-client")
	args = append(args, "-o", cli)
	run(repo, "swiftc", args...)
	writeSharedFixture(t, filepath.Join(root, "app"), "cmd/native-server/main.go", nativeServerSource)
	serverBin := filepath.Join(scratch, "server")
	run(filepath.Join(root, "app"), "go", "build", "-o", serverBin, "./cmd/native-server")
	server := exec.CommandContext(ctx, serverBin)
	stdout, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Process.Kill(); server.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("server did not report origin")
	}
	origin := scanner.Text()
	if !strings.HasPrefix(origin, "http://127.0.0.1:") {
		t.Fatalf("origin=%q", origin)
	}
	cmd := exec.CommandContext(ctx, cli, origin)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated native calls: %v\n%s", err, out)
	}
	if string(out) != "typed native round trip verified\n" {
		t.Fatalf("native output=%q", out)
	}
	response, err := http.Get(origin + "/counts")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var counts map[string]int
	if err := json.NewDecoder(response.Body).Decode(&counts); err != nil {
		t.Fatal(err)
	}
	// ude9z8 was recorded from the installed Kit 3.0.0 hash implementation.
	want := map[string]int{"GET /_app/remote/ude9z8/read": 1, "POST /_app/remote/ude9z8/echo": 1, "GET /_app/remote/ude9z8/wide": 1, "POST /_app/remote/ude9z8/signedInput": 1, "POST /_app/remote/ude9z8/numberInput": 1}
	want["GET /_app/remote/ude9z8/counter"] = 3
	want["POST /_app/remote/ude9z8/increment"] = 1
	want["POST /_app/remote/ude9z8/ignoreCounter"] = 1
	want["POST /_app/remote/ude9z8/forgetCounter"] = 1
	if len(counts) != len(want) {
		t.Fatalf("requests=%v; want %v", counts, want)
	}
	for key, count := range want {
		if counts[key] != count {
			t.Fatalf("requests=%v; want %v", counts, want)
		}
	}
}

const nativeSwiftConsumer = `import Foundation
import SKGoNative
@main struct Probe {
static func main() async throws {
    let client = try RemoteClient(origin: CommandLine.arguments[1])
    let api = NativeAPI(core:client)
    let original = try await api.read()
    precondition(original.Text == "Native voice transcript: café 😀")
    precondition(original.optional == nil && original.Nullable == nil)
    precondition(original.State == .OpenAlias && original.State == .Open)
    precondition(original.Values == [1,2] && original.Fixed == [7,9])
    precondition(original.Choices.isEmpty && original.optionalChoice == nil)
    precondition(original.Time == "2026-10-08T01:02:03.123456789Z")
    precondition(original.First.name == "first" && original.Second.label == "second")
    if case .Note(let note) = original.Choice { precondition(note.Text == "hello") } else { fatalError("wrong union") }
    let input = Envelope(Text:original.Text,optional:"present",Nullable:"also present",State:.Closed,
        Values:[9007199254740991],Fixed:[7,9],Time:original.Time,Choice:original.Choice,
        optionalChoice:original.Choice,Choices:[.Count(Count(Value:3))],Node:Node(Name:"root",Next:original.Node),First:original.First,Second:original.Second)
    let result = try await api.echo(input)
    precondition(result.optional == "present" && result.Nullable == "also present" && result.State == .Closed)
    precondition(result.Node.Name == "root" && result.Node.Next?.Name == "leaf" && result.Node.Next?.Next == nil)
    precondition(result.Values == [9007199254740991])
    if case .Note(let note)? = result.optionalChoice { precondition(note.Text == "hello") } else { fatalError("wrong optional union") }
    if case .Count(let count) = result.Choices[0] { precondition(count.Value == 3) } else { fatalError("wrong union slice") }
    do { _ = try await api.wideInput(UInt64.max); fatalError("sent unsafe integer") } catch is NativeModelError {}
    do { _ = try await api.wideInput(9007199254740993); fatalError("sent unsafe wire integer") } catch is NativeModelError {}
    let signed = try await api.signedInput(-9007199254740991)
    precondition(signed == -9007199254740991)
    let number = try await api.numberInput(1.25)
    precondition(number == 1.25)
    do { _ = try await api.signedInput(-9007199254740993); fatalError("sent unsafe signed integer") } catch is NativeModelError {}
    do { _ = try await api.numberInput(Double.infinity); fatalError("sent infinity") } catch is NativeModelError {}
    do { _ = try await api.numberInput(Double.nan); fatalError("sent NaN") } catch is NativeModelError {}
    do { _ = try await api.wide(); fatalError("received unsafe integer") } catch is RemoteError {}
    let counter = try await api.retainCounter()
    let same = try await api.retainCounter()
    let initial = try await counter.value()
    precondition(initial == 1)
    let shared = try await same.value()
    precondition(shared == 1)
    let acknowledged = try await api.increment(1, updates:[counter.update])
    precondition(acknowledged == 2)
    let updated = try await same.snapshot()
    precondition(updated.ready && !updated.loading && updated.current == 2 && updated.error == nil)
    let ignored = try await api.ignoreCounter(updates:[counter.update])
    precondition(ignored == "ignored")
    let stillValid = try await counter.snapshot()
    precondition(stillValid.current == 2 && stillValid.error == nil)
    let forgot = try await api.forgetCounter(updates:[counter.update])
    precondition(forgot == "forgot")
    let unhandled = try await same.snapshot()
    precondition(unhandled.current == 2 && unhandled.error == .remote(400,"Requested update was not handled by the remote function"))
    let refreshed = try await same.refresh()
    precondition(refreshed == 2)
    await counter.release()
    await same.release()
    precondition(initial == 1 && updated.current == 2)
    let uncached = try await api.counter()
    precondition(uncached == 2)
    await client.shutdown()
    print("typed native round trip verified")
}
}
`
