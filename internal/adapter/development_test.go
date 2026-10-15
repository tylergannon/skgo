package adapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
)

// Kit owns discovery and both node tables. Adding a lexically earlier page
// changes an existing page's index; checking both literals catches stale pairing.
func TestDevelopmentSnapshotUsesLiveKitGraphAtGoHandler(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(dir, "web")
	work := filepath.Join(dir, "session")
	os.MkdirAll(web, 0755)
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(web, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(web, "node_modules")); err != nil {
		t.Fatal(err)
	}
	write("package.json", `{"private":true,"type":"module"}`)
	write("vite.config.js", `import {sveltekit} from '@sveltejs/kit/vite';import skgo from '@skgo/sveltekit-adapter';export default {plugins:[sveltekit({adapter:skgo(),experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}})]};`)
	write("src/app.html", `<!doctype html><html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>`)
	write("src/routes/+layout.svelte", `<script>import Child from "./Child.svelte";import {PUBLIC_RECEIPT,PUBLIC_DYNAMIC} from "$app/env/public";let{children}=$props()</script><aside>{PUBLIC_RECEIPT} {PUBLIC_DYNAMIC}</aside><svelte:boundary>{#if true}<Child />{/if}</svelte:boundary><main>{@render children()}</main>`)
	write("src/routes/Child.svelte", `<span>CHILD_ANCHOR</span>`)
	write("static/robots.txt", "User-agent: *\nDisallow: /STATIC_7751\n")
	write("go.mod", "module adapter-environment-proof\n\ngo 1.27.1\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => "+filepath.ToSlash(root)+"\ntool github.com/tylergannon/skgo/cmd/skgo\n")
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(goSum))
	write("skgo.env.json", `{"fields":[{"name":"PUBLIC_RECEIPT","field":"Receipt","type":"string","public":true,"static":true},{"name":"SECRET_RECEIPT","field":"Secret","type":"string","static":true},{"name":"PUBLIC_DYNAMIC","field":"Dynamic","type":"string","public":true}]}`)
	write(".env.development", "PUBLIC_RECEIPT=ENV_8826\nSECRET_RECEIPT=PRIVATE_9943\n")
	write("src/env.ts", `import {defineEnvVars} from '@sveltejs/kit/env';import {values}from'../.svelte-kit/skgo-env-values.js';export const variables=defineEnvVars({PUBLIC_RECEIPT:{public:true,static:true,schema:()=>values.PUBLIC_RECEIPT},SECRET_RECEIPT:{public:false,static:true,schema:()=>values.SECRET_RECEIPT},PUBLIC_DYNAMIC:{public:true,static:false,schema:()=>values.PUBLIC_DYNAMIC}});`)

	write("src/routes/+error.svelte", `<script>import{page}from'$app/state';</script><h1>GRAPH_ERROR {page.status}</h1>`)
	stub := `export function load(){throw new Error('NODE_APPLICATION_LOAD_EXECUTED')}`
	page := func(title string) string {
		return `<script>let{data}=$props();</script><h1>` + title + `</h1><p>Go value: {data.value}</p>`
	}
	write("src/routes/+page.svelte", page("HOME"))
	write("src/routes/+page.server.ts", stub)
	write("src/routes/z-existing/+page.svelte", page("EXISTING"))
	write("src/routes/z-existing/+page.server.ts", stub)
	generated := func(added bool) {
		loads := []string{"src/routes/+page.server.ts", "src/routes/z-existing/+page.server.ts"}
		if added {
			loads = append(loads, "src/routes/a-added/+page.server.ts")
		}
		b, _ := json.Marshal(map[string]any{"remotes": []string{}, "actions": []string{}, "endpoints": map[string]any{}, "loads": loads, "prerender": map[string]string{"root": dir, "package": "./unused"}})
		write("skgo.remotes.json", string(b))
	}
	generated(false)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tidy := exec.CommandContext(ctx, "go", "mod", "tidy")
	tidy.Dir = web
	if out, e := tidy.CombinedOutput(); e != nil {
		t.Fatalf("fixture dependencies: %v %s", e, out)
	}
	var output bytes.Buffer
	readyPath := filepath.Join(work, "ready.json")
	cmd := exec.CommandContext(ctx, "vp", "dev", "--host", "127.0.0.1", "--port", "0")
	cmd.Dir = web
	cmd.Env = append(os.Environ(), "SKGO_DEV_ORIGIN=http://127.0.0.1:56899", "SKGO_DEV_DIR="+work, "SKGO_DEV_READY="+readyPath, "GOFLAGS=-mod=mod", "PUBLIC_DYNAMIC=DYNAMIC_3317")
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		if t.Failed() {
			t.Log(output.String())
		}
	}()
	var ready struct{ URL, Instance string }
	for end := time.Now().Add(45 * time.Second); ; {
		b, _ := os.ReadFile(readyPath)
		if json.Unmarshal(b, &ready) == nil && ready.URL != "" {
			break
		}
		if time.Now().After(end) {
			t.Fatal("Vite readiness missing")
		}
		time.Sleep(25 * time.Millisecond)
	}
	initialInstance := ready.Instance
	if initialInstance == "" {
		t.Fatal("missing Vite instance identity")
	}
	initialURL := ready.URL
	client := &http.Client{Timeout: 15 * time.Second}
	// The public client must pass through Vite's resolver so navigation and
	// the running Kit client share one module identity.
	res, err := client.Get(ready.URL + "/__skgo/dev/client.js")
	if err != nil {
		t.Fatal(err)
	}
	clientSource, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(clientSource), "refreshAll") || !strings.Contains(string(clientSource), "skgo:go-ready") {
		t.Fatalf("development client %d: %s", res.StatusCode, clientSource)
	}
	state := func() string {
		t.Helper()
		res, e := client.Get(ready.URL + "/__skgo/dev/state")
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return string(b)
	}
	beforeGoEdit := state()
	write("src/routes/ordinary.go", "package routes\n// ordinary Go edit\n")
	if after := state(); after != beforeGoEdit {
		t.Fatalf("Go-only edit changed frontend revision: %s -> %s", beforeGoEdit, after)
	}
	expectedReceipt := "ENV_8826"
	var generation int
	snapshot := func() http.Handler {
		t.Helper()
		res, e := client.Get(ready.URL + "/__skgo/dev/snapshot")
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("snapshot%d: %s", res.StatusCode, b)
		}
		var s struct {
			Dist, Revision string
			Metadata       json.RawMessage
		}
		if e := json.Unmarshal(b, &s); e != nil {
			t.Fatal(e)
		}
		if len(s.Metadata) == 0 || s.Revision == "" {
			t.Fatalf("missing Kit metadata/revision: %s", b)
		}
		fsys := os.DirFS(s.Dist)
		bundle, e := os.ReadFile(filepath.Join(s.Dist, "ssr/bundle.js"))
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(bundle), "PRIVATE_9943") {
			t.Fatal("private build input was baked into renderer")
		}
		m, e := skgo.ReadManifest(fsys)
		if e != nil {
			t.Fatal(e)
		}
		var specs []*skgo.ServerLoad
		for _, module := range m.Loads {
			value := 1101
			if strings.Contains(module, "z-existing") {
				value = 8426
			}
			if strings.Contains(module, "a-added") {
				value = 7719
			}
			specs = append(specs, skgo.NewServerLoad(skgo.LoadSpec{Module: module, Run: func(context.Context) (any, error) { return map[string]any{"value": value}, nil }}))
		}
		loads, e := skgo.NewLoads(m.LoadConfig("http://127.0.0.1:56899"), specs...)
		if e != nil {
			t.Fatal(e)
		}
		remotes, e := skgo.NewRemotes(m.RemoteConfig("http://127.0.0.1:56899"))
		if e != nil {
			t.Fatal(e)
		}
		type envValues struct {
			Receipt string `env:"PUBLIC_RECEIPT,public,static"`
			Secret  string `env:"SECRET_RECEIPT,static"`
			Dynamic string `env:"PUBLIC_DYNAMIC,public"`
		}
		values, environment, e := skgo.LoadEnvironment[envValues](fsys, func(name string) (string, bool) { return "DYNAMIC_3317", name == "PUBLIC_DYNAMIC" })
		if e != nil {
			t.Fatal(e)
		}
		if values.Receipt != expectedReceipt || values.Secret != "PRIVATE_9943" {
			t.Fatalf("Go environment=%#v", values)
		}
		renderer, e := skgo.NewSSR(fsys, m, loads, remotes, skgo.SSROptions{Runtimes: 2, Environment: environment})
		if e != nil {
			t.Fatal(e)
		}
		h, e := skgo.NewStaticHandler(fsys, skgo.WithSSR(renderer))
		if e != nil {
			t.Fatal(e)
		}
		generation++
		t.Logf("snapshot%d routes%d nodes%d", generation, len(m.Routes), len(m.Nodes))
		return loads.Intercept(h)
	}
	assert := func(h http.Handler, path, literal string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status || !strings.Contains(w.Body.String(), literal) {
			t.Fatalf("%s expected%d %q got%d %s", path, status, literal, w.Code, w.Body.String())
		}
	}
	h := snapshot()
	assert(h, "/z-existing", "CHILD_ANCHOR</span><!---->", 200)
	assert(h, "/z-existing", "ENV_8826", 200)
	assert(h, "/z-existing", "DYNAMIC_3317", 200)
	for _, suffix := range []string{"", "?raw"} {
		privateResponse, e := client.Get(ready.URL + "/@fs" + filepath.ToSlash(filepath.Join(work, "skgo-env-values.js")) + suffix)
		if e != nil {
			t.Fatal(e)
		}
		privateBody, _ := io.ReadAll(privateResponse.Body)
		privateResponse.Body.Close()
		if privateResponse.StatusCode != 500 || !strings.Contains(string(privateBody), "environment build inputs are private") || strings.Contains(string(privateBody), "PRIVATE_9943") {
			t.Fatalf("private environment response%d: %s", privateResponse.StatusCode, privateBody)
		}
	}
	assert(h, "/z-existing", "Go value: 8426", 200)
	assert(h, "/robots.txt", "Disallow: /STATIC_7751", 200)
	write("src/routes/a-added/+page.svelte", page("ADDED"))
	write("src/routes/a-added/+page.server.ts", stub)
	generated(true)
	h = snapshot()
	assert(h, "/a-added", "Go value: 7719", 200)
	assert(h, "/z-existing", "Go value: 8426", 200)
	write("src/routes/a-added/+page.svelte", page("CHANGED"))
	write("static/robots.txt", "User-agent: *\nDisallow: /STATIC_8862\n")
	h = snapshot()
	assert(h, "/a-added", "CHANGED", 200)
	assert(h, "/robots.txt", "Disallow: /STATIC_8862", 200)
	if e := os.RemoveAll(filepath.Join(web, "src/routes/a-added")); e != nil {
		t.Fatal(e)
	}
	generated(false)
	h = snapshot()
	assert(h, "/a-added", "GRAPH_ERROR 404", 404)
	assert(h, "/z-existing", "Go value: 8426", 200)
	beforeEnvironmentEdit := state()
	write(".env.development", "PUBLIC_RECEIPT=ENV_9937\nSECRET_RECEIPT=PRIVATE_9943\n")
	if state() == beforeEnvironmentEdit {
		t.Fatal("dotenv edit did not change frontend revision")
	}
	expectedReceipt = "ENV_9937"
	// Vite itself restarts on dotenv changes. Observe its new actual listener,
	// rather than treating a transient refused connection as a snapshot.
	time.Sleep(500 * time.Millisecond)
	for end := time.Now().Add(10 * time.Second); ; {
		b, _ := os.ReadFile(readyPath)
		_ = json.Unmarshal(b, &ready)
		r, e := client.Get(ready.URL + "/__skgo/dev/state")
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(end) {
			t.Fatal("Vite did not recover after dotenv edit")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if ready.Instance == initialInstance {
		t.Fatal("Vite restart did not change instance identity")
	}
	t.Logf("Vite restart %s/%s -> %s/%s", initialURL, initialInstance, ready.URL, ready.Instance)
	h = snapshot()
	assert(h, "/z-existing", "ENV_9937", 200)
}
