//go:build darwin && skgo_qualification

package sdk_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/devalue/v5"
	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo"
	nativeclient "github.com/tylergannon/skgo/plugins/native-client"
	"github.com/tylergannon/skgo/plugins/native-shell/sdk"
	"github.com/tylergannon/skgo/plugins/native-shell/swiftgen"
)

// Qualify the actual exported SDK: fresh static linkage, generated calls over
// HTTP to the real dispatcher, and the existing Swift lifecycle contracts.
func TestSwiftRemoteClientStaticSDK(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := nativeclient.CopySDK(sdk.Resources(), dir, "native-shell"); err != nil {
		t.Fatal(err)
	}
	run := func(work string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "MISE_TRUSTED_CONFIG_PATHS="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	t.Log(strings.TrimSpace(run(dir, "swift", "--version")))
	t.Log("zig=" + strings.TrimSpace(run(filepath.Join(dir, "core"), "mise", "exec", "--", "zig", "version")))
	run(filepath.Join(dir, "core"), "mise", "exec", "--", "zig", "build", "test", "library")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	var mu sync.Mutex
	text, revision := "", 0
	reads, writes := 0, 0
	getText := func(context.Context) (*devalue.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		reads++
		return devalue.NewObject("text", text, "revision", revision), nil
	}
	query := skgo.NewRemote(skgo.RemoteSpec{Module: "src/native.remote.ts", Name: "getText", Kind: skgo.KindQuery, Fn: getText, Call: func(ctx context.Context, c skgo.Call) (any, error) {
		if c.Present {
			return nil, skgo.Errorf(400, "No argument expected")
		}
		return getText(ctx)
	}})
	command := skgo.NewRemote(skgo.RemoteSpec{Module: "src/native.remote.ts", Name: "acceptText", Kind: skgo.KindCommand, Call: func(ctx context.Context, c skgo.Call) (any, error) {
		if !c.Present || c.Arg != "Native voice transcript: café 😀" {
			return nil, skgo.Errorf(400, "Literal transcript required")
		}
		mu.Lock()
		text = "Native voice transcript: café 😀"
		revision = 1
		writes++
		mu.Unlock()
		if err := skgo.RefreshRequestedNoArg(ctx, getText); err != nil {
			return nil, err
		}
		return devalue.NewObject("text", "Native voice transcript: café 😀", "revision", 1), nil
	}})
	handler, err := skgo.NewRemotes(skgo.RemoteConfig{Base: "/native", Origin: origin}, query, command)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	name := tg.Name{PackagePath: "fixture", Name: "TextState"}
	defs := tg.Definitions{{Name: name, Type: &tg.Object{Fields: []tg.Field{
		{GoName: "Text", JSONName: "text", Value: &tg.Required{Type: &tg.Scalar{Kind: tg.String}}},
		{GoName: "Revision", JSONName: "revision", Value: &tg.Required{Type: &tg.Scalar{Kind: tg.Int}}},
	}}}}
	generated, err := swiftgen.Generate(defs, []tg.Type{&tg.Ref{Target: name}, &tg.Scalar{Kind: tg.String}}, []swiftgen.Call{
		{Name: "getText", ID: query.ID(), Kind: "query", Input: -1, Output: 0},
		{Name: "acceptText", ID: command.ID(), Kind: "command", Input: 1, Output: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"Generated.swift": generated.Source, "main.swift": []byte(swiftConsumer)} {
		if err := os.WriteFile(filepath.Join(dir, "swift/Sources/RemoteCLI", name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"swift", "test", "--package-path", filepath.Join(dir, "swift"), "--scratch-path", filepath.Join(dir, "build"), "--jobs", "4"}
	// CLT ships Testing's macro plug-in but does not automatically load it.
	compiler := strings.TrimSpace(run(dir, "xcrun", "--find", "swiftc"))
	macro := filepath.Join(filepath.Dir(filepath.Dir(compiler)), "lib/swift/host/plugins/testing/libTestingMacros.dylib")
	if _, err := os.Stat(macro); err == nil {
		args = append(args, "-Xswiftc", "-load-plugin-library", "-Xswiftc", macro)
	}
	t.Log(run(dir, args...))
	bin := strings.TrimSpace(run(dir, "swift", "build", "--package-path", filepath.Join(dir, "swift"), "--scratch-path", filepath.Join(dir, "build"), "--show-bin-path"))
	executable := filepath.Join(bin, "skgo-swift-remote")
	links := run(dir, "otool", "-L", executable)
	if strings.Contains(links, "libskgo_native_core") {
		t.Fatalf("Swift dynamically links native core: %s", links)
	}
	out := run(dir, executable, origin)
	t.Log("consumer output:\n" + out)
	for _, want := range []string{`initial: text="" revision=0`, `confirmed: text="Native voice transcript: café 😀" revision=1`, "Swift: retained refresh and reset PASS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q", want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if text != "Native voice transcript: café 😀" || revision != 1 || reads < 4 || writes != 1 {
		t.Fatal(fmt.Sprintf("server text=%q revision=%d reads=%d writes=%d", text, revision, reads, writes))
	}
	t.Log("Swift: generated consumer used static Zig archive and real handler; all SDK lifecycle contracts passed")
}

const swiftConsumer = `import SKGoNative
import Foundation

@main struct Consumer {
static func main() async throws {
    let client = RemoteClient(origin: CommandLine.arguments[1], base: "/native")
    let api = NativeAPI(core: client)
    let initial = try await api.getText()
    precondition(initial.text == "" && initial.revision == 0)
    print("initial: text=\"\(initial.text)\" revision=\(initial.revision)")
    let retained = try await api.retainGetText()
    let value = try await retained.value()
    precondition(value.text == "" && value.revision == 0)
    _ = try await api.acceptText("Native voice transcript: café 😀", updates: [retained.update])
    let updated = try await retained.snapshot()
    precondition(updated.current?.text == "Native voice transcript: café 😀" && updated.current?.revision == 1)
    let refreshed = try await retained.refresh()
    precondition(refreshed.text == "Native voice transcript: café 😀" && refreshed.revision == 1)
    try await client.resetSession()
    do { _ = try await retained.value(); fatalError("Reset preserved old lease") }
    catch RemoteError.releasedQuery {}
    let confirmed = try await api.getText()
    precondition(confirmed.text == "Native voice transcript: café 😀" && confirmed.revision == 1)
    print("confirmed: text=\"\(confirmed.text)\" revision=\(confirmed.revision)")
    await client.shutdown()
    print("Swift: retained refresh and reset PASS")
}
}
`
