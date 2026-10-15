//go:build skgo_browser_experiment

package hotreload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo/generate"
	"golang.org/x/tools/go/packages"
)

// Run only with the explicit build tag. Owns both child processes for a bounded
// interactive browser experiment; it is not a normal regression test.
func TestNativeViteBrowserExperiment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	t.Setenv("GOWORK", "off")
	t.Setenv("CGO_ENABLED", "0")
	root, _ := filepath.Abs("../..")
	work := t.TempDir()
	work, _ = filepath.EvalSymlinks(work)
	app := filepath.Join(work, "application")
	fixture := filepath.Join(root, "ephemeral/experiments/20261015-vite-cooperation/browser")
	write := func(name string, b []byte) {
		t.Helper()
		path := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) []byte {
		t.Helper()
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	write("go.mod", []byte(strings.ReplaceAll(strings.ReplaceAll(string(read(filepath.Join(root, "example/go.mod"))), "module github.com/tylergannon/skgo/example", "module example.com/nativefixture"), "=> ../", "=> "+filepath.ToSlash(root))))
	write("go.sum", read(filepath.Join(root, "go.sum")))
	write("internal/app/locals.go", []byte("package app\ntype Locals struct{}\n"))
	write("web/package.json", read(filepath.Join(root, "example/web/package.json")))
	write("web/src/app.html", read(filepath.Join(root, "example/web/src/app.html")))
	write("web/vite.config.ts", read(filepath.Join(fixture, "vite.config.ts")))
	write("web/src/routes/orders/+page.svelte", read(filepath.Join(fixture, "page.svelte")))
	write("web/src/routes/+layout.ts", []byte("export const ssr=false;\n"))
	write("web/src/routes/orders/orders.remote.go", []byte(orderSource))
	if err := os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(app, "web/node_modules")); err != nil {
		t.Fatal(err)
	}
	tc, err := PrepareToolchain(ctx, "go", filepath.Join(work, "toolchain"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		raw, err := tc.command(ctx, app, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, raw)
		}
		return raw
	}
	run("mod", "download")
	prepared, err := generate.Prepare(generate.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/nativefixture/internal/app", LocalsType: "Locals", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err = prepared.Run(); err != nil {
		t.Fatal(err)
	}
	links, err := os.ReadDir(filepath.Join(app, "generated/links"))
	if err != nil || len(links) != 1 {
		t.Fatalf("links: %v %v", links, err)
	}
	rel := filepath.Join("generated/links", links[0].Name())
	dir := filepath.Join(app, rel)
	pkg := "example.com/nativefixture/" + filepath.ToSlash(rel)
	run("mod", "tidy")
	pp, err := packages.Load(&packages.Config{Context: ctx, Dir: app, Env: tc.command(ctx, app).Env, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}, pkg)
	if err != nil || len(pp) != 1 {
		t.Fatalf("load: %v %v", pp, err)
	}
	sources := map[string][]byte{}
	for _, file := range pp[0].CompiledGoFiles {
		sources[file] = read(file)
	}
	lowered, err := Lower(pp[0], sources)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range lowered.Host {
		write(filepath.Join(rel, name), raw)
	}
	write("cmd/main.go", []byte(strings.ReplaceAll(browserHost, "PACKAGE", pkg)))
	if _, err = tc.PrepareLoader(ctx, app, filepath.Join(work, "native")); err != nil {
		t.Fatal(err)
	}
	run("mod", "tidy")
	bin := filepath.Join(app, "host")
	run("build", "-ldflags=-s=false", "-o", bin, "./cmd")
	write("importcfg", run("list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", pkg))
	baseline := map[string][]byte{}
	for file, raw := range sources {
		baseline[file] = raw
	}
	baseline[filepath.Join(dir, "skgo_hot.go")] = []byte("package orders\n")
	host := exec.CommandContext(ctx, bin)
	host.Dir = app
	in, _ := host.StdinPipe()
	out, _ := host.StdoutPipe()
	host.Stderr = os.Stderr
	if err = host.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); host.Process.Kill(); host.Wait() }()
	dec, enc := json.NewDecoder(out), json.NewEncoder(in)
	var ready struct {
		URL string
		PID int
	}
	if err = dec.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	backend, _ := url.Parse(ready.URL)
	goProxy := httputil.NewSingleHostReverseProxy(backend)
	viteURL, _ := url.Parse("http://127.0.0.1:56141")
	viteProxy := httputil.NewSingleHostReverseProxy(viteURL)
	vite := exec.CommandContext(ctx, "vp", "dev")
	vite.Dir = filepath.Join(app, "web")
	vite.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	vite.Cancel = func() error { return syscall.Kill(-vite.Process.Pid, syscall.SIGKILL) }
	vite.Stdout = os.Stdout
	vite.Stderr = os.Stderr
	if err = vite.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { syscall.Kill(-vite.Process.Pid, syscall.SIGKILL); vite.Wait() }()
	var mu sync.Mutex
	revision := 0
	mux := http.NewServeMux()
	mux.Handle("/_app/remote/", goProxy)
	mux.HandleFunc("/__experiment/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("stopping"))
		go func() { time.Sleep(time.Second); cancel() }()
	})
	mux.HandleFunc("/__experiment/frontend", func(w http.ResponseWriter, r *http.Request) {
		file := filepath.Join(app, "web/src/routes/orders/+page.svelte")
		raw, e := os.ReadFile(file)
		if e == nil {
			e = os.WriteFile(file, bytes.ReplaceAll(raw, []byte("FRONTEND_ONE"), []byte("FRONTEND_TWO")), 0644)
		}
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.Write([]byte("edited Svelte source"))
	})
	mux.HandleFunc("/__experiment/update", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		revision++
		start := time.Now()
		value := r.URL.Query().Get("value")
		mode := r.URL.Query().Get("mode")
		if value != "broken" {
			if _, e := strconv.Atoi(value); e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
		}
		changed := map[string][]byte{}
		for file, raw := range baseline {
			changed[file] = raw
		}
		changed[filepath.Join(dir, "orders.remote.go")] = []byte(strings.Replace(orderSource, "return total", "return total*0+"+value, 1))
		candidate, e := tc.Compile(ctx, app, pkg, filepath.Join(app, "importcfg"), filepath.Join(app, fmt.Sprintf("revision-%d", revision)), baseline, changed)
		prepared := time.Since(start)
		if e != nil {
			http.Error(w, e.Error(), 422)
			t.Logf("candidate preparation rejected revision=%d duration=%s: %v", revision, prepared, e)
			return
		}
		e = enc.Encode(candidate)
		var installed struct {
			PID   int
			Error string
		}
		if e == nil {
			e = dec.Decode(&installed)
		}
		if e != nil || installed.Error != "" {
			http.Error(w, fmt.Sprintf("install: %v %s", e, installed.Error), 500)
			return
		}
		if installed.PID != ready.PID {
			http.Error(w, "host PID changed", 500)
			return
		}
		publication := time.Now()
		notify := fmt.Sprintf("http://127.0.0.1:56141/__published?mode=%s&published=%d", url.QueryEscape(mode), publication.UnixMilli())
		res, e := http.Get(notify)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		t.Logf("installed revision=%d value=%s pid=%d prepare=%s install=%s mode=%s", revision, value, installed.PID, prepared, publication.Sub(start)-prepared, mode)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"value": value, "pid": installed.PID, "prepareMS": prepared.Milliseconds(), "published": publication.UnixMilli()})
	})
	mux.Handle("/", viteProxy)
	listener, err := net.Listen("tcp", "127.0.0.1:56142")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux}
	defer server.Close()
	go server.Serve(listener)
	t.Logf("READY public=http://127.0.0.1:56142/orders hostPID=%d fixture=%s; POST /__experiment/stop ends experiment", ready.PID, app)
	<-ctx.Done()
}

const browserHost = `package main
import("encoding/json";"net/http/httptest";"os";"github.com/tylergannon/skgo";"example.com/nativefixture/generated";orders "PACKAGE")
func main(){
 if err:=orders.SkgoHotInit(8);err!=nil{panic(err)}
 registry,err:=skgo.NewRemotes(skgo.RemoteConfig{Origin:"http://127.0.0.1:56142"},generated.Remotes()...);if err!=nil{panic(err)}
 server:=httptest.NewServer(orders.SkgoHotHandler(registry));defer server.Close()
 enc,dec:=json.NewEncoder(os.Stdout),json.NewDecoder(os.Stdin)
 enc.Encode(struct{URL string;PID int}{server.URL,os.Getpid()})
 for{var c struct{Object string;Storage []string;Reclaim bool};if err:=dec.Decode(&c);err!=nil{return};e:="";if err:=orders.SkgoHotInstall(c.Object,c.Storage,c.Reclaim);err!=nil{e=err.Error()};enc.Encode(struct{PID int;Error string}{os.Getpid(),e})}
}
`
