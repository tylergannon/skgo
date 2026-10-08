package native_test

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo"
)

// The CLI uses real HTTP to the production registry and dispatcher. No mock
// protocol server, source-derived expectation, or separately maintained harness.
func TestZigRemoteCalls(t *testing.T) {
	buildContext, cancelBuild := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancelBuild()
	prefix := t.TempDir()
	build := exec.CommandContext(buildContext, "mise", "exec", "--", "zig", "build", "test", "install", "--prefix", prefix)
	build.Dir = "core"
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native CLI (Zig 0.17.0 required): %v\n%s", err, out)
	}
	// A cold Linux compiler/libc build is separate from the actual HTTP budget.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cli := filepath.Join(prefix, "bin", "skgo-remote")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	var mu sync.Mutex
	text := ""
	revision := 0
	commandCount, queryCount := 0, 0
	command := skgo.NewRemote(skgo.RemoteSpec{
		Module: "src/native.remote.ts", Name: "acceptText", Kind: skgo.KindCommand,
		Call: func(_ context.Context, call skgo.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			commandCount++
			if call.Arg != "Native voice transcript: café 😀" || !call.Present {
				return nil, skgo.Errorf(400, "Literal transcript was not received")
			}
			text = "Native voice transcript: café 😀"
			revision = 1
			return devalue.NewObject("text", text, "revision", revision), nil
		},
	})
	query := skgo.NewRemote(skgo.RemoteSpec{
		Module: "src/native.remote.ts", Name: "getText", Kind: skgo.KindQuery,
		Call: func(_ context.Context, call skgo.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			queryCount++
			if call.Present {
				return nil, skgo.Errorf(400, "No argument expected")
			}
			return devalue.NewObject("text", text, "revision", revision), nil
		},
	})
	failure := skgo.NewRemote(skgo.RemoteSpec{
		Module: "src/native.remote.ts", Name: "conflict", Kind: skgo.KindQuery,
		Call: func(context.Context, skgo.Call) (any, error) { return nil, skgo.Errorf(409, "Revision conflict") },
	})
	redirect := skgo.NewRemote(skgo.RemoteSpec{
		Module: "src/native.remote.ts", Name: "signIn", Kind: skgo.KindQuery,
		Call: func(context.Context, skgo.Call) (any, error) {
			r, err := skgo.NewRedirect(303, "/sign-in")
			if err != nil {
				return nil, err
			}
			return nil, r
		},
	})
	handler, err := skgo.NewRemotes(skgo.RemoteConfig{Base: "/native", Origin: origin, Version: "native-test"}, command, query, failure, redirect)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	call := func(id, kind, argument string) (string, error) {
		t.Helper()
		out, err := exec.CommandContext(ctx, cli, origin, "/native", id, kind, argument).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	for _, step := range []struct{ id, kind, argument, want string }{
		{query.ID(), "query", "-1", `[{"text":1,"revision":2},"",0]`},
		{command.ID(), "command", `["Native voice transcript: café 😀"]`, `[{"text":1,"revision":2},"Native voice transcript: café 😀",1]`},
		{query.ID(), "query", "-1", `[{"text":1,"revision":2},"Native voice transcript: café 😀",1]`},
	} {
		got, err := call(step.id, step.kind, step.argument)
		if err != nil {
			t.Fatalf("%s: %v\n%s", step.kind, err, got)
		}
		if got != step.want {
			t.Fatalf("%s result = %s; want %s", step.kind, got, step.want)
		}
	}
	mu.Lock()
	if commandCount != 1 || queryCount != 2 {
		t.Fatalf("calls: command=%d query=%d; want 1,2", commandCount, queryCount)
	}
	mu.Unlock()
	for _, step := range []struct{ id, argument, want string }{
		{failure.ID(), "-1", "remote_error (409): Revision conflict"},
		{query.ID(), "[null]", "remote_error (400): No argument expected"},
		{"unknown/call", "-1", "remote_error (404)"},
		{redirect.ID(), "-1", "redirect (200): /sign-in"},
		{command.ID(), "-1", "http_error (405)"},
	} {
		got, err := call(step.id, "query", step.argument)
		if err == nil || !strings.Contains(got, step.want) {
			t.Fatalf("error = %v, output = %s; want %s", err, got, step.want)
		}
	}
}
