package adapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
)

// Kit owns discovery, rendering and its live node graph. Route edits must
// retain the independently supplied Go values across the public Go proxy.
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
	work, err := os.MkdirTemp("/tmp", "skgo-adapter-dev-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	work, err = filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	viteSocket, goSocket := filepath.Join(work, "vite.sock"), filepath.Join(work, "go.sock")
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", viteSocket)
	}}
	defer transport.CloseIdleConnections()
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: "skgo.invalid"})
	proxy.Transport = transport
	public := httptest.NewServer(proxy)
	defer public.Close()
	var appMu sync.RWMutex
	var application http.Handler
	var closeNative func()
	defer func() {
		if closeNative != nil {
			closeNative()
		}
	}()
	goListener, err := net.Listen("unix", goSocket)
	if err != nil {
		t.Fatal(err)
	}
	goServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appMu.RLock()
		handler := application
		appMu.RUnlock()
		if handler == nil {
			http.Error(w, "Go application not initialized", 503)
			return
		}
		handler.ServeHTTP(w, r)
	})}
	go goServer.Serve(goListener)
	defer goServer.Close()
	const secret = "native-development-test-secret"
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
	// Kit 3 writes project-specific $app types and tsconfigs in node_modules.
	// Share installed dependencies, but give these outputs their own directory.
	dependencies := filepath.Join(root, "example/web/node_modules")
	sharedTypes := filepath.Join(dependencies, "$app/types/index.d.ts")
	originalTypes, err := os.ReadFile(sharedTypes)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		after, err := os.ReadFile(sharedTypes)
		if err != nil || !bytes.Equal(after, originalTypes) {
			t.Errorf("fixture changed real application's Kit route types: %v", err)
		}
	}()
	modules := filepath.Join(web, "node_modules")
	if err := os.MkdirAll(modules, 0755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "$app" || strings.HasPrefix(name, "$app.") || name == ".cache" || strings.HasPrefix(name, ".vite") {
			continue
		}
		if err := os.Symlink(filepath.Join(dependencies, name), filepath.Join(modules, name)); err != nil {
			t.Fatal(err)
		}
	}
	packageJSON, err := os.ReadFile(filepath.Join(root, "example/web/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	write("package.json", string(packageJSON))
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

	write("src/routes/+error.svelte", `<script>import{page}from'$app/state';</script><h1>GRAPH_ERROR {page.status}</h1><p>{page.error.message} {page.error.code}</p>`)
	stub := "// Code generated by skgo. DO NOT EDIT.\nexport const load = async (event) => {throw new Error('NODE_APPLICATION_LOAD_EXECUTED')};"
	write("src/hooks.server.ts", "// Code generated by skgo. DO NOT EDIT.\nexport {};")
	page := func(title string) string {
		return `<script>let{data}=$props();</script><h1>` + title + `</h1><p>Go value: {data.value}</p>`
	}
	write("src/routes/+page.svelte", page("HOME"))
	write("src/routes/+page.server.ts", stub)
	write("src/routes/z-existing/+page.svelte", page("EXISTING"))
	write("src/routes/z-existing/+page.server.ts", stub)
	// Invalid arguments must cross the generated Vite callback and reach Go's
	// hook without reaching application code, including a full App.Error body.
	const validationModule = "src/routes/validation.remote.ts"
	write(validationModule, "// Code generated by skgo. DO NOT EDIT.\nimport {query} from '$app/server';export const rejected=query('unchecked',async(arg)=>{throw new Error('NODE_REMOTE_BODY_EXECUTED')});")
	write("src/routes/validation/+page.svelte", `<script>import {rejected} from '../validation.remote';const result=await rejected(42);</script><h1>SHOULD_NOT_RENDER {result}</h1>`)
	var validationCalls, validationHooks atomic.Int32
	validationRemote := skgo.NewRemote(skgo.RemoteSpec{Kind: skgo.KindQuery, Module: validationModule, Name: "rejected", Call: func(_ context.Context, call skgo.Call) (any, error) {
		if _, ok := call.Arg.(string); !ok {
			return nil, skgo.BadRequest(errors.New("PRIVATE_VALIDATION_DIAGNOSTIC_771"))
		}
		validationCalls.Add(1)
		return "accepted", nil
	}})
	generated := func(added bool) {
		loads := []string{"src/routes/+page.server.ts", "src/routes/z-existing/+page.server.ts"}
		if added {
			loads = append(loads, "src/routes/a-added/+page.server.ts")
		}
		b, _ := json.Marshal(map[string]any{"remotes": []string{validationRemote.ID()}, "actions": []string{}, "endpoints": map[string]any{}, "loads": loads, "prerender": map[string]string{"root": dir, "package": "./unused"}})
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
	cmd := exec.CommandContext(ctx, filepath.Join(web, "node_modules/.bin/vp"), "exec", "node", "--input-type=module", "-e", `await import(new URL('./skgo-adapter/dev-host.js',import.meta.resolve('@skgo/sveltekit-adapter')));`)
	cmd.Dir = web
	cmd.Env = append(os.Environ(), "SKGO_DEV_ORIGIN="+public.URL, "SKGO_DEV_DIR="+work, "SKGO_DEV_READY="+readyPath, "SKGO_DEV_VITE_SOCKET="+viteSocket, "SKGO_DEV_GO_SOCKET="+goSocket, "SKGO_DEV_SERVICE_SECRET="+secret, "GOFLAGS=-mod=mod", "PUBLIC_DYNAMIC=DYNAMIC_3317")
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
	var ready struct{ Socket, Instance string }
	var listenErr error
	for end := time.Now().Add(45 * time.Second); ; {
		b, _ := os.ReadFile(readyPath)
		if json.Unmarshal(b, &ready) == nil && ready.Socket == viteSocket {
			// configureServer publishes its epoch before dev-host binds the
			// external listener. Wait for that listener, without retrying or
			// concealing any HTTP errors in the assertions below.
			var connection net.Conn
			connection, listenErr = net.DialTimeout("unix", viteSocket, time.Second)
			if listenErr == nil {
				connection.Close()
				break
			}
			if !errors.Is(listenErr, syscall.ENOENT) && !errors.Is(listenErr, syscall.ECONNREFUSED) {
				t.Fatalf("Vite listener readiness: %v", listenErr)
			}
		}
		if time.Now().After(end) {
			t.Fatalf("Vite readiness missing: listener error %v", listenErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
	localTypes := filepath.Join(modules, "$app/types/index.d.ts")
	resolvedTypes, err := filepath.EvalSymlinks(localTypes)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedTypes != localTypes {
		t.Fatalf("fixture Kit types escaped local node_modules: %s", resolvedTypes)
	}
	types, err := os.ReadFile(localTypes)
	if err != nil || !strings.Contains(string(types), "/z-existing") {
		t.Fatalf("missing fixture-specific local route types: %s (%v)", types, err)
	}
	initialInstance := ready.Instance
	if initialInstance == "" {
		t.Fatal("missing Vite instance identity")
	}
	initialSocket := ready.Socket
	client := &http.Client{Timeout: 15 * time.Second}
	// The public client must pass through Vite's resolver so navigation and
	// the running Kit client share one module identity.
	res, err := client.Get(public.URL + "/__skgo/dev/client.js")
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
		res, e := client.Get(public.URL + "/__skgo/dev/state")
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
	var environmentArtifact string
	snapshot := func() {
		t.Helper()
		res, e := client.Get(public.URL + "/__skgo/dev/snapshot")
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
		environmentArtifact = filepath.Join(s.Dist, "env.json")
		fsys := os.DirFS(s.Dist)
		development := skgo.NewNativeDevelopment(secret, viteSocket)
		if _, err := os.Stat(filepath.Join(s.Dist, "ssr/bundle.js")); !os.IsNotExist(err) {
			t.Fatal("native development unexpectedly published a renderer bundle")
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
		loads, e := skgo.NewLoads(m.LoadConfig(public.URL), specs...)
		if e != nil {
			t.Fatal(e)
		}
		remoteConfig := m.RemoteConfig(public.URL)
		remoteConfig.HandleError = func(_ context.Context, caught skgo.CaughtError) map[string]any {
			validationHooks.Add(1)
			if caught.Kind != "validation" || caught.Status != 400 || caught.Message != "Bad Request" || len(caught.Issues) != 1 || caught.Issues[0].Message != "PRIVATE_VALIDATION_DIAGNOSTIC_771" {
				t.Errorf("validation caught=%+v", caught)
			}
			return map[string]any{"status": 422, "message": "PUBLIC_VALIDATION_882", "code": "PUBLIC_CODE_993"}
		}
		remotes, e := skgo.NewRemotes(remoteConfig, validationRemote)
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
		renderer, e := development.NewSSR(fsys, m, loads, remotes, skgo.SSROptions{Runtimes: 2, Environment: environment})
		if e != nil {
			t.Fatal(e)
		}
		h, e := development.NewStaticHandler(fsys, skgo.WithSSR(renderer))
		if e != nil {
			t.Fatal(e)
		}
		generation++
		t.Logf("snapshot%d routes%d nodes%d", generation, len(m.Routes), len(m.Nodes))
		handler, cleanup, e := development.Handler(loads.Intercept(remotes.Intercept(h)))
		if e != nil {
			t.Fatal(e)
		}
		if closeNative != nil {
			closeNative()
		}
		appMu.Lock()
		application = handler
		appMu.Unlock()
		closeNative = cleanup
	}
	assert := func(path, literal string, status int) {
		t.Helper()
		var body []byte
		var got int
		var readErr error
		for deadline := time.Now().Add(5 * time.Second); ; {
			response, err := client.Get(public.URL + path)
			if err == nil {
				body, readErr = io.ReadAll(response.Body)
				response.Body.Close()
				got = response.StatusCode
				if strings.Contains(string(body), "PRIVATE_9943") {
					t.Fatalf("private environment input appeared in %s", path)
				}
				if readErr == nil && got == status && strings.Contains(string(body), literal) {
					return
				}
			} else {
				readErr = err
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s expected%d %q got%d %s (%v)", path, status, literal, got, body, readErr)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	snapshot()
	validationResponse, err := client.Get(public.URL + "/validation")
	if err != nil {
		t.Fatal(err)
	}
	validationBody, err := io.ReadAll(validationResponse.Body)
	validationResponse.Body.Close()
	if err != nil || validationResponse.StatusCode != 422 || !strings.Contains(string(validationBody), "PUBLIC_VALIDATION_882 PUBLIC_CODE_993") || strings.Contains(string(validationBody), "PRIVATE_VALIDATION_DIAGNOSTIC_771") || validationCalls.Load() != 0 || validationHooks.Load() != 1 {
		t.Fatalf("validation HTTP%d calls=%d hooks=%d body=%s (%v)", validationResponse.StatusCode, validationCalls.Load(), validationHooks.Load(), validationBody, err)
	}
	assert("/z-existing", "CHILD_ANCHOR</span>", 200)
	assert("/z-existing", "ENV_8826", 200)
	assert("/z-existing", "DYNAMIC_3317", 200)
	firstEnvironmentArtifact := environmentArtifact
	assertPrivate := func(file string) {
		t.Helper()
		for _, suffix := range []string{"", "?raw"} {
			privateResponse, e := client.Get(public.URL + "/@fs" + filepath.ToSlash(file) + suffix)
			if e != nil {
				t.Fatal(e)
			}
			privateBody, _ := io.ReadAll(privateResponse.Body)
			privateResponse.Body.Close()
			if privateResponse.StatusCode != 403 || !strings.Contains(string(privateBody), "environment build inputs are private") || strings.Contains(string(privateBody), "PRIVATE_9943") {
				t.Fatalf("private environment response%d: %s", privateResponse.StatusCode, privateBody)
			}
		}
	}
	assertPrivate(filepath.Join(work, "skgo-env-values.js"))
	assertPrivate(firstEnvironmentArtifact)
	alias := filepath.Join(dir, "private-work-alias")
	if err := os.Symlink(work, alias); err != nil {
		t.Fatal(err)
	}
	assertPrivate(filepath.Join(alias, "skgo-env-values.js"))
	relativeArtifact, err := filepath.Rel(work, firstEnvironmentArtifact)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate(filepath.Join(alias, relativeArtifact))

	assert("/z-existing", "Go value: 8426", 200)
	assert("/robots.txt", "Disallow: /STATIC_7751", 200)
	write("src/routes/a-added/+page.svelte", page("ADDED"))
	write("src/routes/a-added/+page.server.ts", stub)
	generated(true)
	snapshot()
	assert("/a-added", "Go value: 7719", 200)
	assert("/z-existing", "Go value: 8426", 200)
	write("src/routes/a-added/+page.svelte", page("CHANGED"))
	write("static/robots.txt", "User-agent: *\nDisallow: /STATIC_8862\n")
	snapshot()
	assert("/a-added", "CHANGED", 200)
	assert("/robots.txt", "Disallow: /STATIC_8862", 200)
	if e := os.RemoveAll(filepath.Join(web, "src/routes/a-added")); e != nil {
		t.Fatal(e)
	}
	generated(false)
	snapshot()
	assert("/a-added", "GRAPH_ERROR 404", 404)
	assert("/z-existing", "Go value: 8426", 200)
	beforeEnvironmentEdit := state()
	write(".env.development", "PUBLIC_RECEIPT=ENV_9937\nSECRET_RECEIPT=PRIVATE_9943\n")
	if state() == beforeEnvironmentEdit {
		t.Fatal("dotenv edit did not change frontend revision")
	}
	expectedReceipt = "ENV_9937"
	// Vite reloads its environment inside the existing private middleware host.
	// Its new configuration epoch is announced while the private socket stays stable.
	for end := time.Now().Add(10 * time.Second); ; {
		b, _ := os.ReadFile(readyPath)
		if json.Unmarshal(b, &ready) == nil && ready.Instance != initialInstance {
			if ready.Instance == "" || ready.Socket != initialSocket {
				t.Fatalf("invalid dotenv restart readiness: %+v", ready)
			}
			r, e := client.Get(public.URL + "/__skgo/dev/state")
			if e != nil {
				t.Fatal(e)
			}
			r.Body.Close()
			if r.StatusCode != 200 {
				t.Fatalf("dotenv restart state: HTTP%d", r.StatusCode)
			}
			break
		}
		if time.Now().After(end) {
			t.Fatal("Vite did not recover after dotenv edit")
		}
		time.Sleep(25 * time.Millisecond)
	}
	snapshot()
	assert("/z-existing", "ENV_9937", 200)
	assertPrivate(firstEnvironmentArtifact)
	assertPrivate(environmentArtifact)
}
