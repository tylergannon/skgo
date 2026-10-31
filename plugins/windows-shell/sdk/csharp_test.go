//go:build skgo_qualification

package sdk_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/devalue/v5"
	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo"
	nativeclient "github.com/tylergannon/skgo/plugins/native-client"
	"github.com/tylergannon/skgo/plugins/windows-shell/csharpgen"
	clientsdk "github.com/tylergannon/skgo/plugins/windows-shell/sdk"
)

// One build, one standalone process, real HTTP to SKGo's production dispatcher.
// The consumer asserts literal results, including lifecycle with late real HTTP
// responses. Go owns server/consumer budgets and cleanup.
func TestCSharpRemoteClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sdk := filepath.Join(dir, "sdk")
	if err := nativeclient.CopySDK(clientsdk.Resources(), filepath.Join(dir, "sdk"), "windows-shell"); err != nil {
		t.Fatal(err)
	}
	build := func(work string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "MISE_TRUSTED_CONFIG_PATHS="+sdk)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	t.Log("zig=" + strings.TrimSpace(build(filepath.Join(sdk, "core"), "mise", "exec", "--", "zig", "version")))
	t.Log("dotnet=" + strings.TrimSpace(build(dir, "dotnet", "--version")))
	build(filepath.Join(sdk, "core"), "mise", "exec", "--", "zig", "build", "shared", "--prefix", filepath.Join(sdk, "core", "zig-out"))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	var mu sync.Mutex
	text := ""
	revision := 0
	queries := 0
	headers := 0
	redirectTargets := 0
	sessionCalls := 0
	getText := func(context.Context) (*devalue.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		queries++
		return devalue.NewObject("text", text, "revision", revision), nil
	}
	const module = "src/native.remote.ts"
	query := csharpQuery(module, "getText", getText)
	command := csharpCommand(module, "acceptText", func(ctx context.Context, arg string) (*devalue.Object, error) {
		if arg != "Native voice transcript: café 😀" {
			return nil, skgo.Errorf(400, "Literal transcript was not received")
		}
		mu.Lock()
		text = "Native voice transcript: café 😀"
		revision = 1
		mu.Unlock()
		if err := skgo.RefreshRequestedNoArg(ctx, getText); err != nil {
			return nil, err
		}
		mu.Lock()
		defer mu.Unlock()
		return devalue.NewObject("text", text, "revision", revision), nil
	})
	ignore := csharpCommandNoArg(module, "ignoreUpdates", func(ctx context.Context) (string, error) { return "ignored", skgo.IgnoreRequestedNoArg(ctx, getText) })
	unhandled := csharpCommandNoArg(module, "unhandledUpdates", func(context.Context) (string, error) { return "unhandled", nil })
	failure := csharpQuery(module, "conflict", func(context.Context) (string, error) { return "", skgo.Errorf(409, "Revision conflict") })
	redirect := csharpQuery(module, "signIn", func(context.Context) (string, error) {
		r, err := skgo.NewRedirect(303, "/sign-in")
		if err != nil {
			return "", err
		}
		return "", r
	})
	presence := skgo.NewRemote(skgo.RemoteSpec{Module: module, Name: "presence", Kind: skgo.KindQuery, Call: func(_ context.Context, c skgo.Call) (any, error) {
		if !c.Present {
			return "omitted", nil
		}
		if c.Arg == nil {
			return "null", nil
		}
		return nil, skgo.Errorf(400, "Only omission or null expected")
	}})
	session := csharpQuery(module, "sessionNumber", func(context.Context) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		sessionCalls++
		return sessionCalls, nil
	})
	httpRedirect := csharpCommandNoArg(module, "httpRedirect", func(context.Context) (string, error) { return "unexpected", nil })
	remotes, err := skgo.NewRemotes(skgo.RemoteConfig{Base: "/native", Origin: origin, Version: "native-csharp-test"}, query, command, ignore, unhandled, failure, redirect, presence, session, httpRedirect)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Header.Get("X-Native-Consumer") == "csharp" {
			headers++
		}
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/"+httpRedirect.ID()) {
			http.Redirect(w, r, "/replayed", 307)
			return
		}
		if r.URL.Path == "/replayed" {
			mu.Lock()
			redirectTargets++
			mu.Unlock()
			http.Error(w, "command replayed", 500)
			return
		}
		remotes.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	name := tg.Name{PackagePath: "fixture", Name: "TextState"}
	defs := tg.Definitions{{Name: name, Type: &tg.Object{Fields: []tg.Field{{GoName: "Text", JSONName: "text", Value: &tg.Required{Type: &tg.Scalar{Kind: tg.String}}}, {GoName: "Revision", JSONName: "revision", Value: &tg.Required{Type: &tg.Scalar{Kind: tg.Int}}}}}}}
	roots := []tg.Type{&tg.Ref{Target: name}, &tg.Scalar{Kind: tg.String}, &tg.Scalar{Kind: tg.Int}}
	calls := []csharpgen.Call{{Name: "getText", ID: query.ID(), Kind: "query", Input: -1, Output: 0}, {Name: "acceptText", ID: command.ID(), Kind: "command", Input: 1, Output: 0}, {Name: "ignoreUpdates", ID: ignore.ID(), Kind: "command", Input: -1, Output: 1}, {Name: "unhandledUpdates", ID: unhandled.ID(), Kind: "command", Input: -1, Output: 1}, {Name: "conflict", ID: failure.ID(), Kind: "query", Input: -1, Output: 1}, {Name: "signIn", ID: redirect.ID(), Kind: "query", Input: -1, Output: 1}, {Name: "sessionNumber", ID: session.ID(), Kind: "query", Input: -1, Output: 2}, {Name: "httpRedirect", ID: httpRedirect.ID(), Kind: "command", Input: -1, Output: 1}}
	generated, err := csharpgen.Generate(defs, roots, calls)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "csharp", "RemoteCLI", "Generated.cs"), generated.Source, 0600); err != nil {
		t.Fatal(err)
	}
	build(filepath.Join(sdk, "csharp", "RemoteCLI"), "dotnet", "build", "--nologo")
	out := build(dir, "dotnet", filepath.Join(sdk, "csharp", "RemoteCLI", "bin", "Debug", "net10.0", "RemoteCLI.dll"), origin, presence.ID())
	t.Log("consumer output:\n" + out)
	for _, want := range []string{`initial: text="" revision=0`, `confirmed: text="Native voice transcript: café 😀" revision=1`, "retained: shared value, observed command update, refresh, ignored/unhandled updates PASS", "errors: remote 409, Kit redirect, HTTP 307 without replay, omitted/null PASS", "lifecycle: cancellation releases caller; shutdown closes pending/future work PASS", "lifecycle: reset rejects pending query/command, closes observation, discards old completions PASS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing consumer witness %q", want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if revision != 1 || text != "Native voice transcript: café 😀" || redirectTargets != 0 || headers == 0 || queries < 4 || sessionCalls != 2 {
		t.Fatalf("server state text=%q revision=%d replays=%d headers=%d queries=%d session=%d", text, revision, redirectTargets, headers, queries, sessionCalls)
	}
	t.Log(fmt.Sprintf("server: literal mutation confirmed, command replays=0, application headers received; host=%s/%s", runtime.GOOS, runtime.GOARCH))
}

func csharpQuery[T any](module, name string, fn func(context.Context) (T, error)) *skgo.Remote {
	return skgo.NewRemote(skgo.RemoteSpec{Module: module, Name: name, Kind: skgo.KindQuery, Fn: fn, Call: func(ctx context.Context, c skgo.Call) (any, error) {
		if c.Present {
			return nil, skgo.Errorf(400, "No argument expected")
		}
		return fn(ctx)
	}})
}
func csharpCommandNoArg[T any](module, name string, fn func(context.Context) (T, error)) *skgo.Remote {
	return skgo.NewRemote(skgo.RemoteSpec{Module: module, Name: name, Kind: skgo.KindCommand, Fn: fn, Call: func(ctx context.Context, c skgo.Call) (any, error) {
		if c.Present {
			return nil, skgo.Errorf(400, "No argument expected")
		}
		return fn(ctx)
	}})
}
func csharpCommand[T any](module, name string, fn func(context.Context, string) (T, error)) *skgo.Remote {
	return skgo.NewRemote(skgo.RemoteSpec{Module: module, Name: name, Kind: skgo.KindCommand, Fn: fn, Call: func(ctx context.Context, c skgo.Call) (any, error) {
		arg, ok := c.Arg.(string)
		if !c.Present || !ok {
			return nil, skgo.Errorf(400, "String expected")
		}
		return fn(ctx, arg)
	}})
}
