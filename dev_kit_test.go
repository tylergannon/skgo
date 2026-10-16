package skgo

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeInternalFetchUsesKitAfterGoCredentialsAndHook(t *testing.T) {
	directory, err := os.MkdirTemp("", "skgo-kit-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "vite.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var kitCalls, goCalls, hookCalls atomic.Int32
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kitCalls.Add(1)
		if r.URL.Path != "/child/__data.json" || r.Header.Get("Authorization") != "literal-auth-547" || r.Header.Get("Cookie") != "session=literal-cookie-227" || r.Header.Get("X-Skgo-Internal-Fetch") != "literal-secret-180" || r.Header.Get("X-Skgo-Fetch-Depth") != "1" {
			http.Error(w, "request metadata lost", 500)
			return
		}
		io.WriteString(w, "literal-initial-411\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			io.WriteString(w, "literal-final-372")
		case <-r.Context().Done():
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	development := NewNativeDevelopment("literal-secret-180", socket)
	defer development.kitTransport.CloseIdleConnections()
	source, _ := http.NewRequest("GET", "http://public.test/parent", nil)
	source.Header.Set("Authorization", "literal-auth-547")
	source.Header.Set("Cookie", "session=literal-cookie-227")
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), devKitTransportKey{}, development))
	defer cancel()
	source = source.WithContext(ctx)
	fetcher := &requestFetcher{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goCalls.Add(1)
		http.Error(w, "Go must not dispatch native child", 500)
	}), hook: func(ctx context.Context, r *http.Request, next Fetch) (*http.Response, error) {
		hookCalls.Add(1)
		r = r.Clone(ctx)
		r.URL.Path = "/child/__data.json"
		return next(r)
	}}
	request, _ := http.NewRequest("GET", "http://public.test/alias", nil)
	response, err := fetcher.fetch(ctx, fetchSource{url: source.URL, request: source, jar: newCookieJarAt(source, "public.test", "/parent", false)}, request, fetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	initial, err := reader.ReadString('\n')
	if err != nil || initial != "literal-initial-411\n" {
		t.Fatalf("initial %q %v", initial, err)
	}
	if response.StatusCode != 200 || kitCalls.Load() != 1 || goCalls.Load() != 0 || hookCalls.Load() != 1 {
		t.Fatalf("status=%d kit=%d go=%d hooks=%d", response.StatusCode, kitCalls.Load(), goCalls.Load(), hookCalls.Load())
	}
	cancel()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(reader); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled child looked complete")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not release native fetch")
	}
}
func TestNativeInternalFetchDepthLimit(t *testing.T) {
	development := NewNativeDevelopment("secret")
	request, _ := http.NewRequest("GET", "http://public.test/recursive", nil)
	_, err := development.kitFetch(withFetchDepth(context.Background(), maxFetchDepth), nil, request)
	if err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("depth limit: %v", err)
	}
}

func TestNativeInternalRemoteProtocolKeepsGoWireOwner(t *testing.T) {
	remote := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "src/fixture.remote.ts", Name: "receipt", Call: func(context.Context, Call) (any, error) { return "literal-go-remote-613", nil }})
	remotes, err := NewRemotes(RemoteConfig{Origin: "http://public.test", Development: true}, remote)
	if err != nil {
		t.Fatal(err)
	}
	development := NewNativeDevelopment("secret")
	development.renderer = &SSR{remotes: remotes}
	ctx := context.WithValue(context.Background(), devKitTransportKey{}, development)
	source, _ := http.NewRequest("GET", "http://public.test/parent", nil)
	request, _ := http.NewRequest("GET", "http://public.test"+remotes.Prefix()+remote.ID()+"/", nil)
	fetcher := &requestFetcher{handler: remotes}
	response, err := fetcher.fetch(ctx, fetchSource{url: source.URL, request: source, jar: newCookieJarAt(source, "public.test", "/parent", false)}, request, fetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), "literal-go-remote-613") {
		t.Fatalf("remote status=%d body=%s err=%v", response.StatusCode, body, err)
	}
}
