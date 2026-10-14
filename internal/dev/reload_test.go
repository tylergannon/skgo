package dev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestViteReceivesLatestBuildAfterStartingLate(t *testing.T) {
	root := t.TempDir()
	delivered := make(chan string, 4)
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nested/__skgo_dev/supervisor" || r.Method != "POST" || r.Header.Get("X-Skgo-Dev-Token") != "literal-control-secret" {
			t.Errorf("wrong control request %s %s token=%q", r.Method, r.URL, r.Header.Get("X-Skgo-Dev-Token"))
			w.WriteHeader(403)
			return
		}
		var body struct{ Diagnostic string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		delivered <- body.Diagnostic
		w.WriteHeader(204)
	}))
	defer vite.Close()
	target, _ := url.Parse(vite.URL)
	s, err := New(Config{Root: root, Web: root, Vite: target, Args: func(string) []string { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.reloadLoop(ctx) }()
	s.reload("old failure")
	s.reload("current compile error")
	if err := os.MkdirAll(filepath.Join(root, ".svelte-kit"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".svelte-kit/skgo-dev-supervisor.json"), []byte(`{"token":"literal-control-secret","base":"/nested/"}`), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-delivered:
		if got != "current compile error" {
			t.Fatalf("delivered stale diagnostic %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missed diagnostic after Vite started")
	}
	s.reload("")
	select {
	case got := <-delivered:
		if got != "" {
			t.Fatalf("recovery=%q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missed ready notification")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reload worker survived cancellation")
	}
}

func TestDiagnosticKeepsOnlyViteClientReachable(t *testing.T) {
	root := t.TempDir()
	const dependency = "/nested/node_modules/pinned-vite/dist/client/env.mjs"
	if err := os.MkdirAll(filepath.Join(root, ".svelte-kit"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".svelte-kit/skgo-dev-supervisor.json"), []byte(`{"token":"fixture","base":"/nested/","clientFiles":["`+dependency+`"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "vite-client-fixture") }))
	defer vite.Close()
	target, _ := url.Parse(vite.URL)
	s, err := New(Config{Root: root, Web: root, Vite: target, Args: func(string) []string { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.built, _ = s.in.fingerprint()
	s.settle(&generation{diagnostic: "literal broken Go <script>"})
	for _, path := range []string{"/nested/@vite/client", "/nested/@vite/env", dependency} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.String() != "vite-client-fixture" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/app", nil))
	for _, want := range []string{"literal broken Go &lt;script&gt;", `src="/nested/@vite/client"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %q: %s", want, w.Body.String())
		}
	}
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/nested/src/+page.svelte", "/nested/@vite/unrelated", dependency + "/other", "/nested/node_modules/pinned-vite/dist/client/other.mjs"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 500 || !strings.Contains(w.Body.String(), "literal broken Go &lt;script&gt;") {
			t.Fatalf("unrelated path bypasses Go diagnostic: %s %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestRestartInputsIncludeMembershipAndPreservedTimestamp(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "generator.go")
	if err := os.WriteFile(file, []byte("package fixture\nconst X=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"*.go", "defaults.json"}
	before, err := fingerprintPaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package fixture\nconst X=2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := fingerprintPaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("same-size timestamp-preserving edit ignored")
	}
	if err := os.WriteFile(filepath.Join(root, "new.go"), []byte("package fixture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	membership, err := fingerprintPaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if membership == after {
		t.Fatal("new generator declaration ignored")
	}
	s, err := New(Config{Root: root, Web: root, RestartFiles: paths, Args: func(string) []string { return nil }, Generate: func(context.Context) ([]byte, error) { t.Fatal("stale generator invoked"); return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.restartFingerprint = before
	g := s.build(context.Background())
	if !strings.Contains(g.diagnostic, "Restart the development command") {
		t.Fatalf("missing restart diagnostic: %+v", g)
	}
}

func TestFirstReadyDoesNotQueueABufferedBrowserReload(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:5173")
	s, err := New(Config{Root: t.TempDir(), Web: t.TempDir(), Vite: target, Args: func(string) []string { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.settle(&generation{})
	select {
	case <-s.reloads:
		t.Fatal("initial ready reloads the first arriving visitor")
	default:
	}
	s.setBuilding()
	s.settle(&generation{diagnostic: "literal broken edit"})
	select {
	case got := <-s.reloads:
		if got != "literal broken edit" {
			t.Fatal(got)
		}
	default:
		t.Fatal("edit failure not broadcast")
	}
	s.setBuilding()
	s.settle(&generation{})
	select {
	case got := <-s.reloads:
		if got != "" {
			t.Fatal(got)
		}
	default:
		t.Fatal("repair did not reload browser")
	}
}
