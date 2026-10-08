//go:build darwin

package native_test

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo"
)

func TestSwiftRemoteCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "mise", "exec", "--", "zig", "build", "test", "install")
	build.Dir = "core"
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Zig library: %v\n%s", err, out)
	}
	scratch := t.TempDir()
	swift := exec.CommandContext(ctx, "swift", "test", "--package-path", "swift", "--scratch-path", scratch)
	if out, err := swift.CombinedOutput(); err != nil {
		t.Fatalf("Swift boundary tests: %v\n%s", err, out)
	}
	var mu sync.Mutex
	accepted := false
	commands, queries := 0, 0
	command := skgo.NewRemote(skgo.RemoteSpec{Module: "src/native.remote.ts", Name: "acceptText", Kind: skgo.KindCommand,
		Call: func(_ context.Context, call skgo.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			commands++
			if !call.Present || call.Arg != "Native voice transcript: café 😀" {
				return nil, skgo.Errorf(400, "Literal transcript was not received")
			}
			accepted = true
			return nil, nil
		},
	})
	query := skgo.NewRemote(skgo.RemoteSpec{Module: "src/native.remote.ts", Name: "getText", Kind: skgo.KindQuery,
		Call: func(_ context.Context, call skgo.Call) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			queries++
			if call.Present || !accepted {
				return nil, skgo.Errorf(400, "Expected acknowledged command and omitted query argument")
			}
			return devalue.NewObject("text", "Native voice transcript: café 😀", "revision", 1), nil
		},
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	handler, err := skgo.NewRemotes(skgo.RemoteConfig{Origin: origin, Base: "/native"}, command, query)
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	var redirectMode atomic.Bool
	redirects, replays := 0, 0
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/replayed" {
			mu.Lock()
			replays++
			mu.Unlock()
			w.WriteHeader(500)
			return
		}
		if redirectMode.Load() && r.URL.Path == "/native/_app/remote/"+command.ID() {
			mu.Lock()
			redirects++
			mu.Unlock()
			http.Redirect(w, r, "/replayed", http.StatusTemporaryRedirect)
			return
		}
		handler.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	cli := filepath.Join(scratch, "debug", "skgo-swift-remote")
	out, err := exec.CommandContext(ctx, cli, origin, "/native", command.ID(), query.ID()).CombinedOutput()
	if err != nil {
		t.Fatalf("Swift URLSession calls: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "Native voice transcript: café 😀\nrevision=1" {
		t.Fatalf("Swift result = %q", got)
	}
	mu.Lock()
	if commands != 1 || queries != 1 {
		t.Fatalf("calls command=%d query=%d; want 1,1", commands, queries)
	}
	mu.Unlock()
	redirectMode.Store(true)
	out, err = exec.CommandContext(ctx, cli, origin, "/native", command.ID(), query.ID()).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "http(307") {
		t.Fatalf("HTTP redirect = %v, %s; want refused 307", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	if redirects != 1 || replays != 0 || commands != 1 || queries != 1 {
		t.Fatalf("redirects=%d replays=%d commands=%d queries=%d; want 1,0,1,1", redirects, replays, commands, queries)
	}
}
