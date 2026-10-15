//go:build skgo_cooperation_experiment

package cooperation

import (
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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	skgo "github.com/tylergannon/skgo"
)

func TestCooperation(t *testing.T) {
	root, _ := filepath.Abs("../../../..")
	dir := filepath.Join(root, "ephemeral/experiments/20261015-architecture/cooperation/fixture")
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0755)
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) string {
		t.Helper()
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	write("package.json", read(filepath.Join(root, "example/web/package.json")))
	if e := os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(dir, "node_modules")); e != nil {
		t.Fatal(e)
	}
	config := strings.NewReplacer("ADAPTER_ENV", filepath.Join(root, "internal/adapter/skgo-adapter/env.js"), "WORK_ROOT", root).Replace(read("vite.config.js"))
	write("vite.config.js", config)
	write("src/app.html", `<!doctype html><html><head>%sveltekit.head%<script type="module" src="/@vite/client"></script></head><body><div>%sveltekit.body%</div></body></html>`)
	write("src/routes/+layout.svelte", `<script>let {children}=$props();</script><main>{@render children()}</main>`)
	write("src/routes/+error.svelte", `<script>import {page} from '$app/state';</script><h1>GO_LOAD_ERROR {page.status}</h1><p>{page.error?.message}</p><a href="/">Recover by navigation</a>`)
	page := read("page.svelte")
	write("src/routes/+page.svelte", page)
	write("src/routes/+page.server.ts", `export function load(){throw new Error('NODE_LOAD_MUST_NOT_RUN');}`)
	write("src/routes/api/+server.ts", `export function POST(){throw new Error('NODE_ENDPOINT_MUST_NOT_RUN');}`)
	write("src/lib/value.remote.ts", `import {query} from '$app/server';export const value=query(()=>{throw new Error('NODE_REMOTE_MUST_NOT_RUN');});`)
	write("index.html", "unused fallback")
	write("error.html", "<h1>%sveltekit.status% %sveltekit.error.message%</h1>")
	write("client/static-marker.txt", "GO_STATIC_731")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	log, e := os.Create("vite.log")
	if e != nil {
		t.Fatal(e)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, "vp", "dev")
	cmd.Dir = dir
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); <-done }()
	client := &http.Client{Timeout: 45 * time.Second}
	call := func(path string) string {
		t.Helper()
		res, e := client.Get("http://127.0.0.1:56311/__cooperation/" + path)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("%s: %s", path, b)
		}
		return string(b)
	}
	for deadline := time.Now().Add(25 * time.Second); ; {
		res, e := client.Get("http://127.0.0.1:56311/__cooperation")
		if e == nil {
			res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(e)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("build", call("build"))
	kit, _ := filepath.EvalSymlinks(filepath.Join(dir, "node_modules/@sveltejs/kit"))
	module := "src/routes/+page.server.ts"
	m := skgo.Manifest{AppDir: "_app", Nodes: []string{"", "", module}, Loads: []string{module}, Routes: []skgo.ManifestRoute{{ID: "/", Pattern: "^/$", Page: &skgo.ManifestPage{Layouts: []int{0}, Errors: []int{1}, Leaf: 2}}}, SSR: &skgo.ManifestSSR{Bundle: "bundle.js", Template: "src/app.html", ErrorTemplate: "error.html", Target: "es2022", GlobalName: "__sveltekit_dev", Client: skgo.ManifestClient{Start: "/@fs" + kit + "/src/runtime/client/entry.js", App: "/@fs" + dir + "/.svelte-kit/generated/dev/client/app.js"}, Nodes: []skgo.ManifestSSRNode{{Index: 0, Component: true}, {Index: 1, Component: true}, {Index: 2, Component: true}}}}
	raw, _ := json.Marshal(m)
	write("skgo.manifest.json", string(raw))
	var value atomic.Int64
	value.Store(3000)
	var broken atomic.Bool
	loads, e := skgo.NewLoads(m.LoadConfig("http://127.0.0.1:56313"), skgo.NewServerLoad(skgo.LoadSpec{Module: module, Run: func(context.Context) (any, error) {
		if broken.Load() {
			return nil, fmt.Errorf("GO_LOAD_FAILURE_731")
		}
		return map[string]any{"value": value.Load()}, nil
	}}))
	if e != nil {
		t.Fatal(e)
	}
	remote := skgo.NewRemote(skgo.RemoteSpec{Kind: skgo.KindQuery, Module: "src/lib/value.remote.ts", Name: "value", Call: func(context.Context, skgo.Call) (any, error) { return value.Load(), nil }})
	remotes, e := skgo.NewRemotes(skgo.RemoteConfig{Origin: "http://127.0.0.1:56313"}, remote)
	if e != nil {
		t.Fatal(e)
	}
	var mu sync.RWMutex
	var app http.Handler
	refreshRenderer := func() error {
		renderer, e := skgo.NewSSR(os.DirFS(dir), m, loads, remotes, skgo.SSROptions{Runtimes: 2})
		if e != nil {
			return e
		}
		handler, e := skgo.NewStaticHandler(os.DirFS(dir), skgo.WithSSR(renderer))
		if e != nil {
			return e
		}
		mu.Lock()
		app = remotes.Intercept(loads.Intercept(handler))
		mu.Unlock()
		return nil
	}
	if e = refreshRenderer(); e != nil {
		t.Fatal(e)
	}
	viteURL, _ := url.Parse("http://127.0.0.1:56311")
	proxy := httputil.NewSingleHostReverseProxy(viteURL)
	application := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cooperation-Owner", "Go")
		if r.URL.Path == "/api" {
			body, _ := io.ReadAll(r.Body)
			w.Header().Add("Set-Cookie", "one=1; Path=/")
			w.Header().Add("Set-Cookie", "two=2; Path=/")
			w.Header().Set("X-Received-Method", r.Method)
			w.Header().Set("X-Received-Cookie", r.Header.Get("Cookie"))
			w.WriteHeader(201)
			w.Write(append([]byte("GO_ENDPOINT_731:"), body...))
			return
		}
		mu.RLock()
		h := app
		mu.RUnlock()
		h.ServeHTTP(w, r)
	})
	control := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__control/stop":
			w.Write([]byte("stopping"))
			cancel()
		case "/__control/value":
			v := r.URL.Query().Get("value")
			if v == "broken" {
				broken.Store(true)
			} else {
				broken.Store(false)
				var n int64
				fmt.Sscan(v, &n)
				value.Store(n)
			}
			w.Write([]byte("updated"))
		case "/__control/frontend":
			write("src/routes/+page.svelte", strings.ReplaceAll(page, "FRONTEND_ONE", "FRONTEND_TWO"))
			start := time.Now()
			res, e := client.Get("http://127.0.0.1:56311/__cooperation/rebuild")
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 {
				http.Error(w, string(b), 500)
				return
			}
			if e = refreshRenderer(); e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			t.Logf("frontend artifact+pool=%s builder=%s", time.Since(start), b)
			w.Write([]byte("rebuilt"))
		default:
			http.NotFound(w, r)
		}
	})
	start := func(port string, h http.Handler) {
		t.Helper()
		ln, e := net.Listen("tcp", "127.0.0.1:"+port)
		if e != nil {
			t.Fatal(e)
		}
		s := &http.Server{Handler: h}
		go s.Serve(ln)
		t.Cleanup(func() { s.Close() })
	}
	start("56312", application)
	start("56313", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/__control/") {
			control.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/__data.json" || r.URL.Path == "/api" || r.URL.Path == "/static-marker.txt" || strings.HasPrefix(r.URL.Path, "/_app/remote/") {
			application.ServeHTTP(w, r)
		} else {
			proxy.ServeHTTP(w, r)
		}
	}))
	for _, port := range []string{"56312", "56313", "56311"} {
		for path, want := range map[string]string{"/": "Go load: 3000", "/__data.json": "3000", "/static-marker.txt": "GO_STATIC_731", "/_app/remote/" + remote.ID(): "3000"} {
			res, e := client.Get("http://127.0.0.1:" + port + path)
			if e != nil {
				t.Fatal(e)
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 || !strings.Contains(string(b), want) || res.Header.Get("X-Cooperation-Owner") != "Go" {
				t.Fatalf("port=%s path=%s status=%d body=%s", port, path, res.StatusCode, b)
			}
			t.Logf("verified port=%s path=%s bytes=%d owner=%s", port, path, len(b), res.Header.Get("X-Cooperation-Owner"))
		}
		req, _ := http.NewRequest("POST", "http://127.0.0.1:"+port+"/api", strings.NewReader("BODY_983"))
		req.Header.Set("Cookie", "draft=COOKIE_419")
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 201 || string(b) != "GO_ENDPOINT_731:BODY_983" || len(res.Cookies()) != 2 || res.Header.Get("X-Received-Cookie") != "draft=COOKIE_419" {
			t.Fatalf("proxy fidelity failed %v %s", res, b)
		}
		samples := make([]float64, 30)
		for i := range samples {
			start := time.Now()
			res, e := client.Get("http://127.0.0.1:" + port + "/")
			if e != nil {
				t.Fatal(e)
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			samples[i] = float64(time.Since(start).Microseconds()) / 1000
		}
		sort.Float64s(samples)
		t.Logf("document latency port=%s n=30 p50=%.3fms p95=%.3fms raw_sorted=%v", port, samples[15], samples[28], samples)
	}
	t.Logf("READY Go-edge=http://127.0.0.1:56313 Vite-edge=http://127.0.0.1:56311 remote=%s fixture=%s", remote.ID(), dir)
	if os.Getenv("COOPERATION_INTERACTIVE") == "1" {
		<-ctx.Done()
	}
}
