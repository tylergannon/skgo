package adapter_test

import (
	"bufio"
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
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type emittedTraceSpan struct {
	Name       string            `json:"name"`
	ID         string            `json:"id"`
	Parent     string            `json:"parent"`
	Trace      string            `json:"trace"`
	Attributes map[string]any    `json:"attributes"`
	Status     map[string]any    `json:"status"`
	Events     []json.RawMessage `json:"events"`
}

func readEmittedSpans(t *testing.T, path string) []emittedTraceSpan {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spans []emittedTraceSpan
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var span emittedTraceSpan
		if err := json.Unmarshal(line, &span); err != nil {
			t.Fatal(err, string(line))
		}
		spans = append(spans, span)
	}
	return spans
}
func exportGoSpans(recorder *tracetest.SpanRecorder) []emittedTraceSpan {
	var result []emittedTraceSpan
	for _, span := range recorder.Ended() {
		attrs := map[string]any{}
		for _, attr := range span.Attributes() {
			attrs[string(attr.Key)] = attr.Value.AsInterface()
		}
		result = append(result, emittedTraceSpan{Name: span.Name(), ID: span.SpanContext().SpanID().String(), Parent: span.Parent().SpanID().String(), Trace: span.SpanContext().TraceID().String(), Attributes: attrs})
	}
	return result
}
func assertJoinedTrace(t *testing.T, spans []emittedTraceSpan, endpoint bool) {
	t.Helper()
	byID := map[string]emittedTraceSpan{}
	byName := map[string][]emittedTraceSpan{}
	for _, span := range spans {
		byID[span.ID] = span
		byName[span.Name] = append(byName[span.Name], span)
		if span.Trace != "11111111111111111111111111111111" {
			t.Errorf("lost supplied trace: %s %#v", span.Name, span)
		}
	}
	one := func(name string) emittedTraceSpan {
		t.Helper()
		if len(byName[name]) != 1 {
			t.Fatalf("%s: expected one span, got %d; %#v", name, len(byName[name]), spans)
		}
		return byName[name][0]
	}
	root := one("sveltekit.handle.root")
	resolve := one("sveltekit.resolve")
	outer := one("sveltekit.handle.sequenced.0")
	inner := one("sveltekit.handle.sequenced.1")
	if root.Parent != "2222222222222222" || outer.Parent != root.ID || inner.Parent != outer.ID || resolve.Parent != inner.ID || root.Attributes["receipt.root"] != "ROOT_7819" {
		t.Fatalf("root/resolve %v %v", root, resolve)
	}
	child := one("application.work")
	if endpoint {
		operation := one("sveltekit.endpoint")
		if operation.Parent != resolve.ID || child.Parent != operation.ID || operation.Attributes["receipt.current"] != "ENDPOINT_4361" {
			t.Fatalf("endpoint ancestry/annotations %#v", spans)
		}
		return
	}
	if len(byName["sveltekit.load"]) != 2 {
		t.Fatalf("expected server+universal load: %#v", spans)
	}
	var server, universal emittedTraceSpan
	for _, span := range byName["sveltekit.load"] {
		if span.Attributes["sveltekit.load.node_id"] == "src/routes/+page.server.ts" {
			server = span
		}
		if span.Attributes["sveltekit.load.node_id"] == "src/routes/+page.ts" {
			universal = span
		}
	}
	if server.ID == "" || universal.ID == "" || server.Parent != resolve.ID || universal.Parent != resolve.ID || server.Attributes["receipt.current"] != "LOAD_4361" || universal.Attributes["receipt.universal"] != "UNIVERSAL_5641" || child.Parent != universal.ID {
		t.Fatalf("load/remote ancestry or annotations %#v", spans)
	}
	if len(byName["sveltekit.remote.call"]) != 0 {
		t.Fatalf("SSR direct remote incorrectly acquired HTTP dispatch span %#v", spans)
	}
}

func assertInternalFetchTrace(t *testing.T, spans []emittedTraceSpan) {
	t.Helper()
	var root, nested, load, endpoint emittedTraceSpan
	for _, span := range spans {
		switch {
		case span.Name == "sveltekit.handle.root" && span.Attributes["http.route"] == "/fetch":
			root = span
		case span.Name == "sveltekit.handle.root" && span.Attributes["http.route"] == "/api":
			nested = span
		case span.Name == "sveltekit.load" && span.Attributes["sveltekit.load.node_id"] == "src/routes/fetch/+page.ts":
			load = span
		case span.Name == "sveltekit.endpoint":
			endpoint = span
		}
	}
	if root.ID == "" || nested.ID == "" || load.ID == "" || endpoint.ID == "" || root.Parent != "2222222222222222" || nested.Parent != load.ID || root.Trace != nested.Trace || nested.Attributes["sveltekit.is_sub_request"] != true {
		t.Fatalf("internal fetch lost active context: %#v", spans)
	}
	for _, span := range spans {
		if span.Name == "application.work" && span.Parent != endpoint.ID {
			t.Fatalf("endpoint application span has wrong parent %#v", spans)
		}
	}
}

// One unchanged source tree runs native Kit, builds/prerenders, then runs as
// an actual standalone Go binary. Both SDKs export real spans for comparison.
func TestTracingNativeKitAndProduction(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(dir, "web")
	work, err := os.MkdirTemp("/tmp", "skgo-tracing-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	work, err = filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		file := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	modules := filepath.Join(web, "node_modules")
	os.MkdirAll(modules, 0755)
	dependencies := filepath.Join(repo, "example/web/node_modules")
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
	packageJSON, err := os.ReadFile(filepath.Join(repo, "example/web/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	write("web/package.json", string(packageJSON))
	write("go.mod", "module tracing-fixture\n\ngo 1.27.2\n\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => "+filepath.ToSlash(repo)+"\n")
	write("web/vite.config.js", `import {sveltekit} from '@sveltejs/kit/vite';import skgo from '@skgo/sveltekit-adapter';export default {plugins:[sveltekit({adapter:skgo(),tracing:{server:true},paths:{origin:process.env.ORIGIN??'http://app.test'},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}})]};`)
	write("web/src/app.html", `<!doctype html><html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>`)
	write("web/src/hooks.server.ts", "// Code generated by skgo. DO NOT EDIT.\nexport {};")
	write("web/src/routes/+page.svelte", `<script>let{data}=$props()</script><h1>TRACING_1973 {data.load} {data.remote}</h1>`)
	write("web/src/routes/+page.server.ts", "// Code generated by skgo. DO NOT EDIT.\nexport const load=async(event)=>{throw new Error('NODE_APPLICATION_LOAD_EXECUTED')};")
	write("web/src/routes/+page.ts", `import{value}from'./trace.remote';export async function load({data,tracing}){tracing.root.setAttribute('receipt.universal.root','UNIVERSAL_ROOT_5641');tracing.current.setAttribute('receipt.universal','UNIVERSAL_5641');return {...data,remote:await value()}}`)
	write("web/src/routes/trace.remote.ts", "// Code generated by skgo. DO NOT EDIT.\nimport{query}from'$app/server';export const value=query(async()=>{throw new Error('NODE_APPLICATION_REMOTE_EXECUTED')});")
	write("web/src/routes/failure/+page.svelte", `<h1>Failure fixture</h1>`)
	write("web/src/routes/failure/+page.server.ts", "// Code generated by skgo. DO NOT EDIT.\nexport const load=async(event)=>{throw new Error('NODE_APPLICATION_LOAD_EXECUTED')};export const actions={save:async(event)=>{throw new Error('NODE_APPLICATION_ACTION_EXECUTED')}};")
	write("web/src/routes/panic/+server.ts", "// Code generated by skgo. DO NOT EDIT.\nexport const GET=async(event)=>{throw new Error('NODE_APPLICATION_ENDPOINT_EXECUTED')};")
	write("web/src/routes/fetch/+page.svelte", `<script>let{data}=$props()</script><h1>FETCH_1438 {data.value}</h1>`)
	write("web/src/routes/fetch/+page.ts", `export async function load({fetch,tracing}){tracing.current.setAttribute('receipt.fetch','FETCH_1428');const response=await fetch('/api');return {value:await response.text()}}`)
	write("web/src/routes/api/+server.ts", "// Code generated by skgo. DO NOT EDIT.\nexport const GET=async(event)=>{throw new Error('NODE_APPLICATION_ENDPOINT_EXECUTED')};")
	preload := `if(process.env.SKGO_DEV_ORIGIN){const{NodeTracerProvider}=await import('@opentelemetry/sdk-trace-node');const{SimpleSpanProcessor}=await import('@opentelemetry/sdk-trace-base');const{appendFileSync}=await import('node:fs');const exporter={export(spans,done){for(const s of spans)appendFileSync(process.env.TRACE_FILE,JSON.stringify({name:s.name,id:s.spanContext().spanId,parent:s.parentSpanContext?.spanId??'',trace:s.spanContext().traceId,attributes:s.attributes,status:s.status,events:s.events})+'\n');done({code:0})},shutdown:async()=>{}};new NodeTracerProvider({spanProcessors:[new SimpleSpanProcessor(exporter)]}).register();}`
	write("web/instrumentation.dev.server.js", preload)
	remote := skgo.NewRemote(skgo.RemoteSpec{Module: "src/routes/trace.remote.ts", Name: "value", Kind: skgo.KindQuery, Call: func(ctx context.Context, _ skgo.Call) (any, error) {
		_, span := otel.Tracer("application").Start(ctx, "application.work")
		span.End()
		return "REMOTE_2841", nil
	}})
	generated, _ := json.Marshal(map[string]any{"remotes": []string{remote.ID()}, "loads": []string{"src/routes/+page.server.ts", "src/routes/failure/+page.server.ts"}, "actions": []string{"src/routes/failure/+page.server.ts"}, "endpoints": map[string]any{"/api": []string{"GET"}, "/panic": []string{"GET"}}, "build": map[string]any{"src/hooks.server.ts": map[string]any{"hook": true}, "src/routes/+page.server.ts": map[string]any{"source": "src/routes/page.server.go"}, "src/routes/failure/+page.server.ts": map[string]any{"source": "src/routes/failure/page.server.go"}, "src/routes/panic/+server.ts": map[string]any{"endpoint": true}, "src/routes/api/+server.ts": map[string]any{"endpoint": true}}, "prerender": map[string]string{"root": "..", "package": "./buildservice"}})
	write("web/skgo.remotes.json", string(generated))
	write("buildservice/main.go", `package main
import("context";"net/http";"github.com/tylergannon/skgo")
func main(){err:=skgo.RunPrerenderService(nil,[]*skgo.ServerLoad{skgo.NewServerLoad(skgo.LoadSpec{Module:"src/routes/+page.server.ts",Run:func(context.Context)(any,error){return map[string]any{"load":"LOAD_1937"},nil}})},[]*skgo.Remote{skgo.NewRemote(skgo.RemoteSpec{Module:"src/routes/trace.remote.ts",Name:"value",Kind:skgo.KindQuery,Call:func(context.Context,skgo.Call)(any,error){return "REMOTE_2841",nil}})},skgo.PrerenderServiceOptions{BindRequest:func(next http.Handler)http.Handler{return next},Endpoints:[]*skgo.Endpoint{skgo.NewEndpoint("/api","GET",func(w http.ResponseWriter,r *http.Request){w.Write([]byte("ENDPOINT_6419"))}),skgo.NewEndpoint("/panic","GET",func(w http.ResponseWriter,r *http.Request){w.WriteHeader(500)})}});if err!=nil{panic(err)}}`)
	write("main.go", tracingProductionProgram)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	run := func(cwd string, env []string, name string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(), "GOWORK=off")
		cmd.Env = append(cmd.Env, env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return out
	}
	run(dir, nil, "go", "mod", "tidy")
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(previous); provider.Shutdown(context.Background()) }()
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
	var app http.Handler
	listener, err := net.Listen("unix", goSocket)
	if err != nil {
		t.Fatal(err)
	}
	goServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appMu.RLock()
		h := app
		appMu.RUnlock()
		if h == nil {
			http.Error(w, "not initialized", 503)
			return
		}
		h.ServeHTTP(w, r)
	})}
	go goServer.Serve(listener)
	defer goServer.Close()
	nodeTraceFile := filepath.Join(work, "node-spans.jsonl")
	ready := filepath.Join(work, "ready.json")
	command := exec.CommandContext(ctx, filepath.Join(web, "node_modules/.bin/vp"), "exec", "node", "--input-type=module", "-e", `await import(new URL('./skgo-adapter/dev-host.js',import.meta.resolve('@skgo/sveltekit-adapter')))`)
	command.Dir = web
	command.Env = append(os.Environ(), "GOWORK=off", "SKGO_DEV_ORIGIN="+public.URL, "SKGO_DEV_DIR="+work, "SKGO_DEV_READY="+ready, "SKGO_DEV_VITE_SOCKET="+viteSocket, "SKGO_DEV_GO_SOCKET="+goSocket, "SKGO_DEV_SERVICE_SECRET=tracing-secret", "TRACE_FILE="+nodeTraceFile, "NODE_OPTIONS=--import="+filepath.Join(web, "instrumentation.dev.server.js"))
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		<-done
		if t.Failed() {
			t.Log(output.String())
		}
	}()
	for deadline := time.Now().Add(45 * time.Second); ; {
		if _, err := os.Stat(ready); err == nil {
			connection, err := net.DialTimeout("unix", viteSocket, time.Second)
			if err == nil {
				connection.Close()
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("native Kit readiness timed out")
		}
		time.Sleep(25 * time.Millisecond)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(public.URL + "/__skgo/dev/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct{ Dist string }
	json.NewDecoder(response.Body).Decode(&snapshot)
	response.Body.Close()
	if snapshot.Dist == "" {
		t.Fatal("missing snapshot")
	}
	fsys := os.DirFS(snapshot.Dist)
	m, err := skgo.ReadManifest(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !m.TracingServer {
		t.Fatal("native manifest dropped tracing.server")
	}
	development := skgo.NewNativeDevelopment("tracing-secret", viteSocket)
	loadConfig := m.LoadConfig(public.URL)
	loadConfig.HandleError = func(context.Context, skgo.CaughtError) map[string]any {
		return map[string]any{"message": "PUBLIC_FAILURE_1306", "code": "CUSTOM_6389"}
	}
	loads, err := skgo.NewLoads(loadConfig, skgo.NewServerLoad(skgo.LoadSpec{Module: "src/routes/+page.server.ts", Run: func(ctx context.Context) (any, error) {
		e := skgo.EventFrom(ctx)
		if !e.Tracing.Enabled || e.Tracing.Current.SpanContext().SpanID() != trace.SpanFromContext(ctx).SpanContext().SpanID() {
			return nil, errors.New("native current context mismatch")
		}
		e.Tracing.Root.SetAttributes(attribute.String("receipt.root", "ROOT_7819"))
		e.Tracing.Current.SetAttributes(attribute.String("receipt.current", "LOAD_4361"))
		return map[string]any{"load": "LOAD_1937"}, nil
	}}), skgo.NewServerLoad(skgo.LoadSpec{Module: "src/routes/failure/+page.server.ts", Run: func(context.Context) (any, error) { return nil, errors.New("UNEXPECTED_LOAD_781") }}))
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := skgo.NewRemotes(m.RemoteConfig(public.URL), remote)
	if err != nil {
		t.Fatal(err)
	}
	endpointConfig := m.EndpointConfig(public.URL)
	endpointConfig.HandleError = loadConfig.HandleError
	endpoints, err := skgo.NewEndpoints(endpointConfig, skgo.NewEndpoint("/api", "GET", func(w http.ResponseWriter, r *http.Request) {
		e := skgo.EventFrom(r.Context())
		e.Tracing.Root.SetAttributes(attribute.String("receipt.root", "ROOT_7819"))
		e.Tracing.Current.SetAttributes(attribute.String("receipt.current", "ENDPOINT_4361"))
		_, span := otel.Tracer("application").Start(r.Context(), "application.work")
		span.End()
		io.WriteString(w, "ENDPOINT_6419")
	}), skgo.NewEndpoint("/panic", "GET", func(http.ResponseWriter, *http.Request) { panic("UNEXPECTED_ENDPOINT_783") }))
	if err != nil {
		t.Fatal(err)
	}
	actions, err := skgo.NewActions(skgo.NewPageAction(skgo.ActionSpec{Module: "src/routes/failure/+page.server.ts", Name: "save", Run: func(context.Context) (any, error) { return nil, errors.New("UNEXPECTED_ACTION_782") }}))
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := development.NewSSR(fsys, m, loads, remotes, skgo.SSROptions{Actions: actions})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := development.NewStaticHandler(fsys, skgo.WithSSR(renderer))
	if err != nil {
		t.Fatal(err)
	}
	noop := func(ctx context.Context, event skgo.RequestEvent[struct{}, struct{}], resolve skgo.RequestResolve[struct{}, struct{}]) (*http.Response, error) {
		return resolve(ctx, event)
	}
	hook := skgo.RequestSequence[struct{}, struct{}](noop, noop)
	h := hook.Intercept(m.HandleConfig(), func(*skgo.Event) (struct{}, error) { return struct{}{}, nil }, endpoints.Intercept(loads.Intercept(remotes.Intercept(pages))))
	native, closeNative, err := development.Handler(h)
	if err != nil {
		t.Fatal(err)
	}
	defer closeNative()
	appMu.Lock()
	app = native
	appMu.Unlock()
	visit := func(t *testing.T, base, path, want string) {
		t.Helper()
		request, _ := http.NewRequest("GET", base+path, nil)
		request.Header.Set("Traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
		res, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || !strings.Contains(string(body), want) {
			t.Fatalf("%s: %d %s %v", path, res.StatusCode, body, err)
		}
		if res.Header.Get("X-Skgo-Tracing") != "" {
			t.Fatal("private annotations leaked")
		}
	}
	negative := func(t *testing.T, base, path, method, traceFile, name, diagnostic string) {
		t.Helper()
		request, _ := http.NewRequest(method, base+path, strings.NewReader("value=literal"))
		request.Header.Set("Traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
		request.Header.Set("Accept", "application/json")
		if method == "POST" {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			origin := base
			if base != public.URL {
				origin = "http://app.test"
			}
			request.Header.Set("Origin", origin)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || strings.Contains(string(body), diagnostic) || !strings.Contains(string(body), "PUBLIC_FAILURE_1306") || !strings.Contains(string(body), "CUSTOM_6389") {
			t.Fatalf("private error leaked or custom Go handleError lost: %d %s %v", response.StatusCode, body, err)
		}
		spans := readEmittedSpans(t, traceFile)
		var found *emittedTraceSpan
		for i := range spans {
			span := &spans[i]
			if span.Name == name {
				if found != nil {
					t.Fatal("duplicate operation span", spans)
				}
				found = span
			}
		}
		if found == nil || found.Attributes[name+".result.type"] != "unknown_error" || len(found.Events) != 1 || found.Status["code"] == float64(0) || !strings.Contains(string(found.Events[0]), diagnostic) {
			t.Fatalf("original unexpected classification lost: %#v", spans)
		}
	}

	t.Run("native document", func(t *testing.T) {
		visit(t, public.URL, "/", "TRACING_1973 LOAD_1937 REMOTE_2841")
		for deadline := time.Now().Add(3 * time.Second); ; {
			if raw, _ := os.ReadFile(nodeTraceFile); bytes.Contains(raw, []byte(`"name":"sveltekit.handle.root"`)) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Node SDK did not export root")
			}
			time.Sleep(10 * time.Millisecond)
		}
		spans := append(readEmittedSpans(t, nodeTraceFile), exportGoSpans(recorder)...)
		assertJoinedTrace(t, spans, false)
	})
	recorder.Reset()
	os.WriteFile(nodeTraceFile, nil, 0600)
	t.Run("native endpoint", func(t *testing.T) {
		visit(t, public.URL, "/api", "ENDPOINT_6419")
		for deadline := time.Now().Add(3 * time.Second); ; {
			if raw, _ := os.ReadFile(nodeTraceFile); bytes.Contains(raw, []byte(`"name":"sveltekit.handle.root"`)) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Node SDK did not export endpoint root")
			}
			time.Sleep(10 * time.Millisecond)
		}
		assertJoinedTrace(t, append(readEmittedSpans(t, nodeTraceFile), exportGoSpans(recorder)...), true)
	})
	recorder.Reset()
	os.WriteFile(nodeTraceFile, nil, 0600)
	t.Run("native internal fetch", func(t *testing.T) {
		visit(t, public.URL, "/fetch", "FETCH_1438 ENDPOINT_6419")
		assertInternalFetchTrace(t, append(readEmittedSpans(t, nodeTraceFile), exportGoSpans(recorder)...))
	})

	for _, tc := range []struct{ name, path, method, span, diagnostic string }{
		{"load", "/failure/__data.json", "GET", "sveltekit.load", "UNEXPECTED_LOAD_781"},
		{"action", "/failure?/save", "POST", "sveltekit.form_action", "UNEXPECTED_ACTION_782"},
		{"endpoint", "/panic", "GET", "sveltekit.endpoint", "UNEXPECTED_ENDPOINT_783"},
	} {
		t.Run("native unexpected "+tc.name, func(t *testing.T) {
			recorder.Reset()
			os.WriteFile(nodeTraceFile, nil, 0600)
			negative(t, public.URL, tc.path, tc.method, nodeTraceFile, tc.span, tc.diagnostic)
		})
	}

	// No source changes between native development and production build.
	buildTrace := filepath.Join(work, "build-node-spans.jsonl")
	run(web, []string{"ORIGIN=http://app.test", "NODE_OPTIONS=--import=" + filepath.Join(web, "instrumentation.dev.server.js"), "TRACE_FILE=" + buildTrace}, filepath.Join(web, "node_modules/.bin/vp"), "build")
	if raw, err := os.ReadFile(buildTrace); err == nil && len(raw) > 0 {
		t.Fatal("development SDK initialized during production build", string(raw))
	}
	binary := filepath.Join(work, "standalone")
	run(dir, nil, "go", "build", "-o", binary, ".")
	productionTrace := filepath.Join(work, "go-spans.jsonl")
	production := exec.CommandContext(ctx, binary)
	production.Dir = dir
	production.Env = append(os.Environ(), "TRACE_FILE="+productionTrace)
	stdout, err := production.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var productionErrors bytes.Buffer
	production.Stderr = &productionErrors
	if err := production.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		production.Process.Kill()
		production.Wait()
		if t.Failed() {
			t.Log(productionErrors.String())
		}
	}()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("standalone did not announce address", productionErrors.String())
	}
	base := scanner.Text()
	t.Run("standalone universal document", func(t *testing.T) {
		visit(t, base, "/", "TRACING_1973 LOAD_1937 REMOTE_2841")
		assertJoinedTrace(t, readEmittedSpans(t, productionTrace), false)
	})
	os.WriteFile(productionTrace, nil, 0600)
	t.Run("standalone endpoint", func(t *testing.T) {
		visit(t, base, "/api", "ENDPOINT_6419")
		assertJoinedTrace(t, readEmittedSpans(t, productionTrace), true)
	})
	os.WriteFile(productionTrace, nil, 0600)
	t.Run("standalone internal fetch", func(t *testing.T) {
		visit(t, base, "/fetch", "FETCH_1438 ENDPOINT_6419")
		assertInternalFetchTrace(t, readEmittedSpans(t, productionTrace))
	})
	for _, tc := range []struct{ name, path, method, span, diagnostic string }{
		{"load", "/failure/__data.json", "GET", "sveltekit.load", "UNEXPECTED_LOAD_781"},
		{"action", "/failure?/save", "POST", "sveltekit.form_action", "UNEXPECTED_ACTION_782"},
		{"endpoint", "/panic", "GET", "sveltekit.endpoint", "UNEXPECTED_ENDPOINT_783"},
	} {
		t.Run("standalone unexpected "+tc.name, func(t *testing.T) {
			os.WriteFile(productionTrace, nil, 0600)
			negative(t, base, tc.path, tc.method, productionTrace, tc.span, tc.diagnostic)
		})
	}
}

const tracingProductionProgram = `package main
import("context";"encoding/json";"errors";"fmt";"net";"net/http";"os";"sync";"github.com/tylergannon/skgo";"go.opentelemetry.io/otel";"go.opentelemetry.io/otel/attribute";sdk "go.opentelemetry.io/otel/sdk/trace")
type exporter struct{mu sync.Mutex}
func(e *exporter)ExportSpans(_ context.Context,spans []sdk.ReadOnlySpan)error{e.mu.Lock();defer e.mu.Unlock();f,err:=os.OpenFile(os.Getenv("TRACE_FILE"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{return err};defer f.Close();for _,s:=range spans{attrs:=map[string]any{};for _,a:=range s.Attributes(){attrs[string(a.Key)]=a.Value.AsInterface()};if err:=json.NewEncoder(f).Encode(map[string]any{"name":s.Name(),"id":s.SpanContext().SpanID().String(),"parent":s.Parent().SpanID().String(),"trace":s.SpanContext().TraceID().String(),"attributes":attrs,"status":map[string]any{"code":int(s.Status().Code)},"events":s.Events()});err!=nil{return err}};return nil}
func(*exporter)Shutdown(context.Context)error{return nil}
func main(){otel.SetTracerProvider(sdk.NewTracerProvider(sdk.WithSyncer(&exporter{})));build:=os.DirFS("web/build");m,err:=skgo.ReadManifest(build);if err!=nil{panic(err)};if !m.TracingServer{panic("production manifest lost tracing.server")}
loadConfig:=m.LoadConfig("http://app.test");loadConfig.HandleError=func(context.Context,skgo.CaughtError)map[string]any{return map[string]any{"message":"PUBLIC_FAILURE_1306","code":"CUSTOM_6389"}};loads,err:=skgo.NewLoads(loadConfig,skgo.NewServerLoad(skgo.LoadSpec{Module:"src/routes/+page.server.ts",Run:func(ctx context.Context)(any,error){e:=skgo.EventFrom(ctx);e.Tracing.Root.SetAttributes(attribute.String("receipt.root","ROOT_7819"));e.Tracing.Current.SetAttributes(attribute.String("receipt.current","LOAD_4361"));return map[string]any{"load":"LOAD_1937"},nil}}),skgo.NewServerLoad(skgo.LoadSpec{Module:"src/routes/failure/+page.server.ts",Run:func(context.Context)(any,error){return nil,errors.New("UNEXPECTED_LOAD_781")}}));if err!=nil{panic(err)}
remotes,err:=skgo.NewRemotes(m.RemoteConfig("http://app.test"),skgo.NewRemote(skgo.RemoteSpec{Module:"src/routes/trace.remote.ts",Name:"value",Kind:skgo.KindQuery,Call:func(ctx context.Context,_ skgo.Call)(any,error){_,span:=otel.Tracer("application").Start(ctx,"application.work");span.End();return "REMOTE_2841",nil}}));if err!=nil{panic(err)}
endpointConfig:=m.EndpointConfig("http://app.test");endpointConfig.HandleError=loadConfig.HandleError;endpoints,err:=skgo.NewEndpoints(endpointConfig,skgo.NewEndpoint("/api","GET",func(w http.ResponseWriter,r *http.Request){e:=skgo.EventFrom(r.Context());e.Tracing.Root.SetAttributes(attribute.String("receipt.root","ROOT_7819"));e.Tracing.Current.SetAttributes(attribute.String("receipt.current","ENDPOINT_4361"));_,span:=otel.Tracer("application").Start(r.Context(),"application.work");span.End();w.Write([]byte("ENDPOINT_6419"))}),skgo.NewEndpoint("/panic","GET",func(http.ResponseWriter,*http.Request){panic("UNEXPECTED_ENDPOINT_783")}));if err!=nil{panic(err)}
actions,err:=skgo.NewActions(skgo.NewPageAction(skgo.ActionSpec{Module:"src/routes/failure/+page.server.ts",Name:"save",Run:func(context.Context)(any,error){return nil,errors.New("UNEXPECTED_ACTION_782")}}));if err!=nil{panic(err)};var h http.Handler;renderer,err:=skgo.NewSSR(build,m,loads,remotes,skgo.SSROptions{Actions:actions,Fetch:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){h.ServeHTTP(w,r)})});if err!=nil{panic(err)};pages,err:=skgo.NewStaticHandler(build,skgo.WithSSR(renderer));if err!=nil{panic(err)};noop:=func(ctx context.Context,event skgo.RequestEvent[struct{},struct{}],resolve skgo.RequestResolve[struct{},struct{}])(*http.Response,error){return resolve(ctx,event)};hook:=skgo.RequestSequence[struct{},struct{}](noop,noop);h=hook.Intercept(m.HandleConfig(),func(*skgo.Event)(struct{},error){return struct{}{},nil},endpoints.Intercept(loads.Intercept(remotes.Intercept(pages))));listener,err:=net.Listen("tcp4","127.0.0.1:0");if err!=nil{panic(err)};fmt.Println("http://"+listener.Addr().String());if err:=http.Serve(listener,h);err!=nil{panic(err)}}`
