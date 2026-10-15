package renderer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

func TestRendererDelivery(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(".", "fixture-")
	if err != nil {
		t.Fatal(err)
	}
	dir, _ = filepath.Abs(dir)
	defer os.RemoveAll(dir)
	write := func(name, body string) {
		t.Helper()
		filename := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(root, "example/web/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	write("package.json", string(manifest))
	if err := os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile("vite.config.js")
	if err != nil {
		t.Fatal(err)
	}
	write("vite.config.js", strings.NewReplacer("ADAPTER_ENV", filepath.Join(root, "internal/adapter/skgo-adapter/env.js"), "POLYFILL", filepath.Join(root, "internal/adapter/skgo-adapter/polyfill.js")).Replace(string(config)))
	write("src/app.html", `<!doctype html><html><head>%sveltekit.head%</head><body><div>%sveltekit.body%</div></body></html>`)
	write("src/routes/+layout.svelte", `<script>let { children } = $props();</script><main>{@render children()}</main>`)
	write("src/routes/+page.svelte", `<script>import {label} from './message.js';</script><h1>Component A: {label}</h1>`)
	write("src/routes/message.js", `export const label = 'Dependency A';`)
	write("runner-entry.js", `import {ModuleRunner} from 'vite/module-runner'; import transformed from './runner-module.js'; globalThis.__skgo_ping=()=> 'ok'; const runner = new ModuleRunner({sourcemapInterceptor:false,hmr:false,transport:{invoke:async(payload)=>({result:payload.data.name === 'getBuiltins' ? [] : transformed})}}); globalThis.__skgo_render=()=>{const result={done:false,status:200,head:'',body:''};runner.import('/src/routes/message.js').then(m=>{result.body=String(m.label);result.done=true},e=>{result.failure=String(e);result.done=true});return result};`)
	write("runtime-entry.js", `import {ModuleRunner} from 'vite/module-runner';
 const NativeURL = globalThis.URL; globalThis.URL=class extends NativeURL {constructor(...args){if(typeof args[0]==='string' && args[0].startsWith('file:'))args[0]=args[0].replaceAll('\u0000','%00');try{super(...args)}catch(error){throw new Error('URL constructor inputs '+JSON.stringify(args)+': '+error.message)}}};
 const runner=new ModuleRunner({sourcemapInterceptor:false,hmr:false,transport:{invoke:payload=>__skgo_remote('module',JSON.stringify(payload)).then(JSON.parse)}});
 globalThis.__skgo_ping=()=> 'ok';
 globalThis.__skgo_render = req => {
 let result, failure='';
 const output={};for(const key of ['done','failure','body','head','status','error','redirect','fetched'])Object.defineProperty(output,key,{get:()=>key==='failure'?failure || result?.failure:key==='done'?!!failure || result?.done:result?.[key]});
 runner.import(`+fmt.Sprintf("%q", filepath.Join(root, "internal/adapter/skgo-adapter/entry.js"))+`).then(()=>{result=globalThis.__skgo_render(req)},error=>{failure=error.stack || String(error)});
 return output;
 };`)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	log, err := os.Create(filepath.Join(dir, "vite.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, "vp", "dev", "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--strictPort")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = dir
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		cancel()
		cmd.Wait()
		if t.Failed() {
			raw, _ := os.ReadFile(filepath.Join(dir, "vite.log"))
			t.Log(string(raw))
		}
	}()
	client := &http.Client{Timeout: 45 * time.Second}
	base := fmt.Sprintf("http://127.0.0.1:%d/__experiment", port)
	for deadline := time.Now().Add(20 * time.Second); ; {
		res, err := client.Get(base)
		if err == nil {
			res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Vite did not start:", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	call := func(operation string) time.Duration {
		t.Helper()
		start := time.Now()
		res, err := client.Get(base + operation)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("%s: %s", operation, raw)
		}
		t.Logf("%s wall=%s vite=%s", operation, time.Since(start), raw)
		return time.Since(start)
	}
	render := func(name, want string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		engine, err := ssr.New(name, raw, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		init := time.Since(start)
		request, _ := json.Marshal(ssr.Request{URL: "http://localhost:8080/", RouteID: "/", Status: 200, Branch: []ssr.Node{{Index: 0}, {Index: 1}}})
		start = time.Now()
		result, _, err := engine.Render(context.Background(), "/", request, ssr.Hosts{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Body, want) {
			t.Fatalf("got %q, want %q", result.Body, want)
		}
		t.Logf("%s bytes=%d pool-init=%s render=%s body=%s", name, len(raw), init, time.Since(start), result.Body)
	}
	call("/full")
	render("bundle.js", "Component A: Dependency A")
	write("src/routes/+page.svelte", `<script>import {label} from './message.js';</script><h1>Component B: {label}</h1>`)
	call("/update")
	render("bundle.js", "Component B: Dependency A")
	write("src/routes/message.js", `export const label = 'Dependency B';`)
	call("/update")
	render("bundle.js", "Component B: Dependency B")
	write("src/routes/message.js", `export const label = 'Dependency C';`)
	call("/full")
	render("bundle.js", "Component B: Dependency C")
	call("/runner")
	raw, err := os.ReadFile(filepath.Join(dir, "runner.js"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := ssr.New("runner.js", raw, 2, nil)
	if err != nil {
		t.Logf("Upstream ModuleRunner incompatibility: %v", err)
		lines := strings.Split(string(raw), "\n")
		for i := 718; i < 725 && i < len(lines); i++ {
			t.Logf("runner.js:%d %s", i+1, lines[i])
		}

		raw, err = os.ReadFile(filepath.Join(dir, "runner-cjs.js"))
		if err != nil {
			t.Fatal(err)
		}
		engine, err = ssr.New("runner-cjs.js", raw, 2, nil)
		if err != nil {
			t.Fatalf("CJS lowering incompatibility: %v", err)
		}
	}
	result, _, err := engine.Render(context.Background(), "/", []byte(`{}`), ssr.Hosts{})
	if err != nil {
		t.Fatalf("Upstream ModuleRunner import incompatibility: %v", err)
	}
	if result.Body != "Dependency C" {
		t.Fatalf("runner returned %q", result.Body)
	}
	t.Logf("upstream runner imported transformed module in goja; bytes=%d", len(raw))
	call("/runtime")
	raw, err = os.ReadFile(filepath.Join(dir, "runtime.js"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err = ssr.New("runtime.js", raw, 2, nil)
	if err != nil {
		t.Logf("Actual Kit runtime initialization blocker: %v", err)
		return
	}
	request, _ := json.Marshal(ssr.Request{URL: "http://localhost:8080/", RouteID: "/", Status: 200, Branch: []ssr.Node{{Index: 0}, {Index: 1}}})
	start := time.Now()
	result, _, err = engine.Render(ctx, "/", request, ssr.Hosts{Remote: func(_ context.Context, id, payload string) ([]byte, error) {
		response, err := client.Get(base + "/fetch?" + url.QueryEscape(payload))
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("module fetch: %s", data)
		}
		return data, nil
	}})
	if err != nil {
		t.Logf("Actual Kit runtime blocker after %s: %v", time.Since(start), err)
		return
	}
	if !strings.Contains(result.Body, "Component B: Dependency C") {
		t.Fatalf("runner Kit render got %q", result.Body)
	}
	t.Logf("actual Kit through ModuleRunner rendered in %s: %s", time.Since(start), result.Body)
}
