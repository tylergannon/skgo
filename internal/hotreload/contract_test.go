package hotreload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/generate"
	"github.com/tylergannon/skgo/internal/adapter"
	"golang.org/x/tools/go/packages"
)

// One generated app and one real native host. A literal renderer fixture keeps
// Svelte compilation out of the native reload contract. Every request goes
// through the same generated registry/SSR/endpoint/action composition as its
// production binary. Expectations below are supplied fixture literals.
func TestGeneratedNativeHandlerContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOWORK", "off")
	t.Setenv("CC", filepath.Join(t.TempDir(), "no-C-compiler"))
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(work, "app")
	files := map[string][]byte{}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(app, name)
		if strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum" {
			files[path] = []byte(body)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	tc := sharedNativeToolchain(t, ctx)
	mod, err := os.ReadFile(filepath.Join(root, "example/go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", strings.ReplaceAll(strings.ReplaceAll(string(mod), "module github.com/tylergannon/skgo/example", "module example.com/nativefixture"), "=> ../", "=> "+filepath.ToSlash(root)))
	sums, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(sums))
	write("internal/app/locals.go", `package app; type Locals struct{};type Money struct { Cents int `+"`json:\"cents\"`"+` };func(m Money)Label()string{return "native-money"}`)
	// Generation reads the installed Kit pin, but does not build Kit.
	if err := os.MkdirAll(filepath.Join(app, "web/node_modules/@sveltejs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "example/web/node_modules/@sveltejs/kit"), filepath.Join(app, "web/node_modules/@sveltejs/kit")); err != nil {
		t.Fatal(err)
	}
	write("web/package.json", `{"name":"native-handler-contract","private":true,"type":"module","devDependencies":{"vite-plus":"1.3.0"},"devEngines":{"packageManager":{"name":"pnpm","version":"12.14.0","onFail":"download"}}}`)
	write("web/vite.config.ts", `import {sveltekit} from '@sveltejs/kit/vite';import {defineConfig} from 'vite-plus';import skgo from '@skgo/sveltekit-adapter';export default defineConfig({plugins:[sveltekit({adapter:skgo({precompress:false}),paths:{origin:'http://localhost'},experimental:{remoteFunctions:true}})]});`)
	write("web/src/app.html", `<!doctype html><html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>`)
	write("web/src/hooks.go", `package hooks;import("github.com/tylergannon/skgo";"example.com/nativefixture/internal/app");var _=skgo.Transported[app.Money]("Money")`)
	write("web/src/hooks.ts", `export const transport={Money:{encode:()=>false,decode:(value)=>value}};`)
	write("web/src/routes/orders/+page.svelte", `<script>import "./orders.remote";import "./extra.remote";let {data}=$props();</script><h1>Order total {data.order.total}</h1><p>Again {data.order.again}; initialized {data.order.initializations}; quantity {data.order.quantity}</p>{#if data.fetched}<p>Fetched {data.fetched}</p>{/if}{#await data.later}<p>Waiting</p>{:then value}<p>Deferred {value}</p>{/await}`)
	write("web/src/routes/orders/+page.ts", `export const load=async({data,fetch,url})=>{if(!url.searchParams.has('render'))return data;const response=await fetch('/orders?raw=1&renderHold=1',{headers:{accept:'application/json'}});return {...data,fetched:await response.text()}};`)

	source := strings.Replace(orderSource, "service.quantity=n", `service.quantity=n;if err:=event.SetCookie("quantity","3",skgo.CookieOptions{});err!=nil{return Order{},err}`, 1)
	write("web/src/routes/orders/orders.remote.go", source)
	write("web/src/routes/orders/page.server.go", contractPage)
	write("web/src/routes/orders/server.go", contractEndpoint)
	write("web/src/routes/orders/extra.remote.go", contractRemote)
	write("generate.go", "package nativefixture\n//go:generate go tool skgo generate --web web --out generated --locals-package example.com/nativefixture/internal/app --cache-dir off\n")
	write("cmd/main.go", strings.ReplaceAll(contractHost, "PACKAGE", "example.com/nativefixture/generated/links/onzggl3sn52xizltf5xxezdfojzq"))
	run := func(tool Toolchain, args ...string) []byte {
		t.Helper()
		raw, err := tool.command(ctx, app, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, raw)
		}
		return raw
	}
	run(tc, "mod", "download")
	cfg := generate.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/nativefixture/internal/app", LocalsType: "Locals", CacheDir: "off"}
	// Main imports the not-yet-generated package, so generation only loads its
	// declared routes. It is ordinary production generation throughout.
	if err := generate.Run(cfg); err != nil {
		t.Fatal(err)
	}
	run(tc, "mod", "tidy")
	writeNativeRendererFixture(t, app, write)
	pkgPath := "example.com/nativefixture/generated/links/onzggl3sn52xizltf5xxezdfojzq"
	linkDir := filepath.Join(app, "generated/links/onzggl3sn52xizltf5xxezdfojzq")
	config := &packages.Config{Context: ctx, Dir: app, Env: tc.command(ctx, app).Env, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}
	pkgs, err := packages.Load(config, pkgPath)
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("load route: %v %v", pkgs, err)
	}
	sources := map[string][]byte{}
	for _, name := range pkgs[0].CompiledGoFiles {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sources[name] = raw
	}
	tc.AuthoredPaths = map[string]string{}
	for name := range sources {
		if filepath.Base(name) != "skgo_gen.go" {
			tc.AuthoredPaths[name] = filepath.Join(app, "web/src/routes/orders", filepath.Base(name))
		}
	}
	lowered, err := LowerAuthored(pkgs[0], sources, tc.AuthoredPaths)
	if err != nil {
		t.Fatal(err)
	}
	overlays := map[string][]byte{}
	for name, body := range lowered.Host {
		overlays[filepath.Join(linkDir, name)] = body
	}
	main := strings.ReplaceAll(contractHost, "PACKAGE", pkgPath)
	main = strings.Replace(main, "// HOT_INIT", `if err:=orders.SkgoHotInit(16);err!=nil{panic(err)}; install=func(c candidate)error{return orders.SkgoHotInstall(c.Object,c.Storage,c.Reclaim)}; usage=func()(int,int){return orders.SkgoHotUsage()}`, 1)
	main = strings.Replace(main, "// HOT_FETCH", `fetchConfig.CarryContext=orders.SkgoHotContext`, 1)
	main = strings.Replace(main, "// HOT_HANDLER", `handler=orders.SkgoHotHandler(handler)`, 1)
	overlays[filepath.Join(app, "cmd/main.go")] = []byte(main)
	importcfg := run(tc, "list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", pkgPath)
	write("importcfg", string(importcfg))
	hostTC, err := tc.HostView(ctx, app, filepath.Join(work, "host-view"), overlays, filepath.Join(app, "importcfg"))
	if err != nil {
		t.Fatal(err)
	}
	run(hostTC, "mod", "tidy")
	bin := filepath.Join(work, "host")
	run(hostTC, "build", "-ldflags=-s=false", "-o", bin, "./cmd")

	captured := func(order, page string) map[string][]byte {
		next := map[string][]byte{}
		for name, raw := range sources {
			next[name] = raw
		}
		next[filepath.Join(linkDir, "orders.remote.go")] = []byte(order)
		next[filepath.Join(linkDir, "page.server.go")] = []byte(page)
		endpoint := contractEndpoint
		if strings.Contains(page, "repaired-save:") {
			endpoint = strings.Replace(endpoint, "endpoint:", "endpoint-v2:", 1)
		}
		next[filepath.Join(linkDir, "server.go")] = []byte(endpoint)
		return next
	}
	for name, body := range sources {
		files[name] = body
	}
	retained := &retainedApplication{files: files}
	t.Cleanup(func() {
		if retained.view != nil {
			retained.view.Close()
		}
	})
	compile := func(name, order, page string) (Candidate, error) {
		return tc.compileWithLoader(ctx, app, pkgPath, filepath.Join(app, "importcfg"), filepath.Join(work, name), sources, captured(order, page), []string{"./..."}, retained.Load)
	}
	discount := func(n int) string {
		return strings.Replace(source, "return total", fmt.Sprintf("return total * %d / 100", n), 1)
	}
	c90, err := compile("ninety", discount(90), contractPage)
	if err != nil {
		t.Fatal(err)
	}
	c80, err := compile("eighty", discount(80), strings.Replace(strings.Replace(contractPage, `"saved:%d"`, `"repaired-save:%d"`, 1), "later:=skgo.Resolved", "value.Total++;later:=skgo.Resolved", 1))
	if err != nil {
		t.Fatal(err)
	}
	if c90.Reclaim || c80.Reclaim {
		t.Fatal("deferred/string/boxed handlers must use compatible retention")
	}
	process := startContractHost(t, ctx, app, bin)
	defer process.stop()
	pid := 0
	check := func(method, path, body, want string, status int, headers map[string]string) contractResponse {
		t.Helper()
		got := process.call(contractCommand{Op: "request", Method: method, Path: path, Body: body, Headers: headers})
		if pid == 0 {
			pid = got.PID
		}
		if got.Error != "" || got.Status != status || !strings.Contains(got.Body, want) || got.PID != pid || got.Initializations != 1 {
			t.Fatalf("%s %s: %+v; want status %d body %q stable PID/init1", method, path, got, status, want)
		}
		return got
	}
	html := map[string]string{"Accept": "text/html"}
	jsonHeaders := map[string]string{"Accept": "application/json"}
	check("GET", "/orders", "", "<h1>Order total 3000</h1>", 200, html)
	check("GET", "/orders?raw=1", "", "endpoint:3000", 200, jsonHeaders)
	check("POST", "/orders?/save", "", "saved:3000", 200, map[string]string{"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded", "Origin": "http://localhost"})
	initial := check("POST", "@quantity", `{"payload":"WzNd"}`, `"type":"result"`, 200, map[string]string{"Content-Type": "application/json", "Origin": "http://localhost"})
	if !strings.Contains(initial.Headers.Get("Set-Cookie"), "quantity=3") || !strings.Contains(initial.Body, "/order/") {
		t.Fatalf("cookie/named refresh missing: %+v", initial)
	}
	check("GET", "/orders", "", "<h1>Order total 4200</h1>", 200, html)
	check("GET", "@failure", "", `fixture-teapot`, 200, nil)
	check("GET", "@coins", "", `Money`, 200, nil)
	// Held old execution performs an internal endpoint request only AFTER the
	// new table publishes. Both the endpoint and the unchanged summary stay old.
	process.mustCall(t, contractCommand{Op: "start", Path: "@nested", Headers: map[string]string{"X-Hold": "execution"}})
	process.mustCall(t, contractCommand{Op: "install", Candidate: c90})
	check("GET", "/orders", "", "<h1>Order total 3780</h1>", 200, html)
	held := process.mustCall(t, contractCommand{Op: "release"})
	if !strings.Contains(held.Body, `4200`) || strings.Contains(held.Body, `3780`) {
		t.Fatalf("nested old revision mixed: %+v", held)
	}
	check("GET", "@summary", "", `3780`, 200, nil)
	check("GET", "@coins", "", `3780`, 200, nil)
	check("GET", "@failure", "", `fixture-teapot`, 200, nil)
	afterCommand := check("POST", "@quantity", `{"payload":"WzNd"}`, `"type":"result"`, 200, map[string]string{"Content-Type": "application/json", "Origin": "http://localhost"})
	if !strings.Contains(afterCommand.Headers.Get("Set-Cookie"), "quantity=3") || !strings.Contains(afterCommand.Body, "/order/") {
		t.Fatalf("cookie/named refresh after replacement: %+v", afterCommand)
	}
	// An unrelated live host stream stays open during the next install.
	process.mustCall(t, contractCommand{Op: "stream"})
	// Serialization is inside the outer handler lifetime. The load has returned
	// before the supplied writer blocks and publication proceeds.
	process.mustCall(t, contractCommand{Op: "start", Path: "/orders/__data.json", Headers: map[string]string{"X-Hold": "serialization"}})
	process.mustCall(t, contractCommand{Op: "install", Candidate: c80})
	check("GET", "/orders?raw=1", "", "endpoint-v2:3360", 200, jsonHeaders)
	held = process.mustCall(t, contractCommand{Op: "release"})
	if !strings.Contains(held.Body, `3780`) || strings.Contains(held.Body, `3360`) {
		t.Fatalf("old serialization mixed: %+v", held)
	}
	process.mustCall(t, contractCommand{Op: "endstream"})
	check("GET", "/orders", "", "<h1>Order total 3361</h1>", 200, html)
	check("POST", "/orders?/save", "", "repaired-save:3360", 200, map[string]string{"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded", "Origin": "http://localhost"})
	// Deferred work keeps using its candidate after the load returned, a newer
	// revision published, and GC ran. Its literal is consumed on the real wire.
	process.mustCall(t, contractCommand{Op: "start", Path: "/orders/__data.json?defer=1", Headers: map[string]string{"X-Hold": "deferred"}})
	process.mustCall(t, contractCommand{Op: "install", Candidate: c90})
	held = process.mustCall(t, contractCommand{Op: "release"})
	if !strings.Contains(held.Body, `3360`) || !strings.Contains(held.Body, `"type":"chunk"`) {
		t.Fatalf("deferred candidate not consumed: %+v", held)
	}
	check("GET", "/orders", "", "<h1>Order total 3780</h1>", 200, html)
	// Compile errors and structural changes never enter the host.
	if _, err := compile("broken", strings.Replace(source, "return total", "return (", 1), contractPage); err == nil || !strings.Contains(err.Error(), "orders.remote.go") {
		t.Fatalf("syntax diagnostic lacks authored file: %v", err)
	}
	for _, edit := range []string{strings.Replace(source, "quantity:2", "quantity:7", 1), strings.Replace(source, `return "order"`, `return "different"`, 1), strings.Replace(source, "return total", "go println(1);return total", 1)} {
		_, err := compile("structural", edit, contractPage)
		var rebuild *CompleteBuild
		if !errors.As(err, &rebuild) {
			t.Fatalf("expected full build: %v", err)
		}
	}
	// Exported helper calls outside the dispatch boundary select a full build
	// before object creation, including potential stock-compiler inline copies.
	write("external/caller.go", `package external;import orders "`+pkgPath+`";func Total()int{return orders.ExportedPrice()}`)
	_, err = compile("external-refused", discount(80), contractPage)
	var rebuild *CompleteBuild
	if !errors.As(err, &rebuild) || !strings.Contains(err.Error(), "uninstrumented caller") {
		t.Fatalf("external caller admitted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "external-refused/candidate.o")); !os.IsNotExist(err) {
		t.Fatalf("object created before caller fence: %v", err)
	}
	delete(files, filepath.Join(app, "external/caller.go"))
	if err := os.RemoveAll(filepath.Join(app, "external")); err != nil {
		t.Fatal(err)
	}
	// A forged profile is rejected before object access, preserving the host.
	raw, err := os.ReadFile(c80.Object + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var profile Artifact
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"version", "platform", "identity", "dependencies", "compiler", "package"} {
		changed := profile
		switch field {
		case "version":
			changed.Version = "go1.28.0"
		case "platform":
			changed.Arch = "unsupported"
		case "identity":
			changed.Identity = "different-toolchain"
		case "dependencies":
			changed.Dependencies = "different-archives"
		case "compiler":
			changed.CompilerProfile = "different-settings"
		case "package":
			changed.Package = "example.com/other"
		}
		profileRaw, _ := json.Marshal(changed)
		mismatch := filepath.Join(work, "wrong-object-"+field)
		if err := os.WriteFile(mismatch+".json", profileRaw, 0644); err != nil {
			t.Fatal(err)
		}
		refused := process.call(contractCommand{Op: "install", Candidate: Candidate{Object: mismatch}})
		if !strings.Contains(refused.Error, "profile mismatch") {
			t.Fatalf("%s checked after object access: %+v", field, refused)
		}
	}
	corrupt := filepath.Join(work, "corrupt-object")
	if err := os.WriteFile(corrupt, []byte("wrong object"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corrupt+".json", raw, 0644); err != nil {
		t.Fatal(err)
	}
	refused := process.call(contractCommand{Op: "install", Candidate: Candidate{Object: corrupt}})
	if !strings.Contains(refused.Error, "does not match compiler artifact") {
		t.Fatalf("content checked after loader access: %+v", refused)
	}

	// Panics carry authored file/line through lowering, recovery releases the
	// request lease, and the same initialized process serves healthy/repair work.
	panicPage := strings.Replace(contractPage, `return "healthy"`, `panic("replacement-panic-771")`, 1)
	panicking, err := compile("panic", discount(80), panicPage)
	if err != nil {
		t.Fatal(err)
	}
	process.mustCall(t, contractCommand{Op: "install", Candidate: panicking})
	check("GET", "/orders?panic=1", "", "Internal Error", 500, jsonHeaders)
	panicInfo := process.mustCall(t, contractCommand{Op: "diagnostics"})
	line := 1 + strings.Count(strings.Split(panicPage, `panic("replacement-panic-771")`)[0], "\n")
	if !strings.Contains(panicInfo.Diagnostic, "replacement-panic-771") || !strings.Contains(panicInfo.Diagnostic, fmt.Sprintf("%s:%d", filepath.Join(app, "web/src/routes/orders/page.server.go"), line)) || panicInfo.Consumers != 0 {
		t.Fatalf("panic location/ownership: %+v; want authored line %d", panicInfo, line)
	}
	check("GET", "/orders?raw=1", "", "endpoint:3360", 200, jsonHeaders)
	process.mustCall(t, contractCommand{Op: "install", Candidate: c90})
	check("GET", "/orders?panic=1", "", "endpoint:3780:healthy", 200, jsonHeaders)
	check("GET", "/orders", "", "<h1>Order total 3780</h1>", 200, html)
	// The renderer fixture calls Go fetch during a goja render. Hold its
	// Go HandleFetch hook before internal dispatch, publish, then require both
	// the earlier Go server load and this later fetch to use the old table.
	process.mustCall(t, contractCommand{Op: "start", Path: "/orders?render=1", Headers: map[string]string{"Accept": "text/html", "X-Hold": "render"}})
	process.mustCall(t, contractCommand{Op: "install", Candidate: c80})
	rendered := process.mustCall(t, contractCommand{Op: "release"})
	if rendered.Status != 200 || !strings.Contains(rendered.Body, "<h1>Order total 3780</h1>") || !strings.Contains(rendered.Body, "<p>Fetched endpoint:3780</p>") {
		t.Fatalf("render-time fetch mixed enclosing revision: %+v", rendered)
	}
	check("GET", "/orders?render=1", "", "<p>Fetched endpoint-v2:3360</p>", 200, html)
	process.stop()
	// No restore step: development used private overlays/module declarations.
	// The original authored and generated production tree stays intact.
	productionMod, err := os.ReadFile(filepath.Join(app, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(productionMod), "module example.com/nativefixture\n") || strings.Contains(string(productionMod), "goloader") || strings.Contains(string(productionMod), "purego") {
		t.Fatal("development dependencies escaped private host module")
	}
	if _, err := os.Stat(filepath.Join(linkDir, "skgo_hot.go")); !os.IsNotExist(err) {
		t.Fatalf("host dispatch escaped into generated production tree: %v", err)
	}
	// Compare the authored fixture itself, not another generated baseline.
	authored, err := os.ReadFile(filepath.Join(app, "web/src/routes/orders/orders.remote.go"))
	if err != nil || string(authored) != source {
		t.Fatalf("native build changed authored remote: %v\n%s", err, authored)
	}
	authored, err = os.ReadFile(filepath.Join(app, "cmd/main.go"))
	if err != nil || string(authored) != strings.ReplaceAll(contractHost, "PACKAGE", pkgPath) {
		t.Fatalf("native build changed authored entry point: %v\n%s", err, authored)
	}

}

type contractCommand struct {
	Op, Method, Path, Body string
	Headers                map[string]string
	Candidate              Candidate
}
type contractResponse struct {
	Status, PID, Initializations, Consumers int
	Body, Error, Diagnostic                 string
	Headers                                 mapHeader
}

// http.Header's JSON representation, without putting HTTP in driver setup.
type mapHeader map[string][]string

func (h mapHeader) Get(name string) string {
	for key, value := range h {
		if strings.EqualFold(key, name) && len(value) > 0 {
			return value[0]
		}
	}
	return ""
}

type contractProcess struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	decoder *json.Decoder
	encoder *json.Encoder
	stderr  *bytes.Buffer
}

func startContractHost(t *testing.T, ctx context.Context, dir, bin string) *contractProcess {
	t.Helper()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return &contractProcess{cmd, in, json.NewDecoder(out), json.NewEncoder(in), stderr}
}
func (p *contractProcess) call(c contractCommand) contractResponse {
	if err := p.encoder.Encode(c); err != nil {
		return contractResponse{Error: err.Error()}
	}
	var r contractResponse
	if err := p.decoder.Decode(&r); err != nil {
		r.Error = fmt.Sprintf("host: %v\n%s", err, p.stderr.String())
	}
	return r
}
func (p *contractProcess) mustCall(t *testing.T, c contractCommand) contractResponse {
	t.Helper()
	r := p.call(c)
	if r.Error != "" {
		t.Fatalf("%s: %s\n%s", c.Op, r.Error, p.stderr.String())
	}
	return r
}
func (p *contractProcess) stop() {
	if p.cmd.ProcessState != nil {
		return
	}
	p.in.Close()
	done := make(chan struct{})
	go func() { p.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		p.cmd.Process.Kill()
		<-done
	}
}

const contractPage = `package orders
import("context";"fmt";"github.com/tylergannon/skgo")
type PageData struct{Order Order ` + "`json:\"order\"`" + `;Later skgo.Deferred[int] ` + "`json:\"later\"`" + `}
var DeferredGate *Hold
func ExportedPrice()int{callback:=price;return callback()}
func health()string{return "healthy"}
func pageLoad(event PageRequestEvent)(PageData,error){
 ctx:=event.Context();value,err:=order(ctx);if err!=nil{return PageData{},err}
 later:=skgo.Resolved(price());if event.Request().URL.Query().Has("defer"){later=skgo.Async(ctx,deferredPrice)}
 return PageData{Order:value,Later:later},nil
}
func deferredPrice(ctx context.Context)(int,error){if hold:=DeferredGate;hold!=nil{close(hold.Entered);select{case<-hold.Release:case<-ctx.Done():return 0,ctx.Err()}};return price(),nil}
type SaveResult struct{Receipt string ` + "`json:\"receipt\"`" + `}
func save(ctx context.Context)(SaveResult,error){if err:=skgo.EventFrom(ctx).SetCookie("saved","yes",skgo.CookieOptions{});err!=nil{return SaveResult{},err};return SaveResult{Receipt:fmt.Sprintf("saved:%d",price())},nil}
var _=skgo.Load(pageLoad)
var _=skgo.Action(save)
`
const contractEndpoint = `package orders
import("fmt";"net/http";"github.com/tylergannon/skgo")
func endpoint(w http.ResponseWriter,r *http.Request){suffix:="";if r.URL.Query().Has("panic"){suffix=":"+health()};w.Header().Set("Content-Type","application/json");fmt.Fprintf(w,"endpoint:%d%s",price(),suffix)}
var _=skgo.GET(endpoint)
`
const contractRemote = `package orders
import("context";"io";"net/http";"strconv";"github.com/tylergannon/skgo";"example.com/nativefixture/internal/app")
func nested(ctx context.Context)(Order,error){
 callback:=price;before:=callback();if hold,ok:=ctx.Value("hold").(*Hold);ok{close(hold.Entered);<-hold.Release}
 request,_:=http.NewRequestWithContext(ctx,"GET","http://localhost/orders?raw=1",nil);request.Header.Set("Accept","application/json")
 response,err:=skgo.EventFrom(ctx).Fetch(ctx,request);if err!=nil{return Order{},err};defer response.Body.Close();body,err:=io.ReadAll(response.Body);if err!=nil{return Order{},err};after,err:=strconv.Atoi(string(body)[9:]);if second:=callback();second!=after{return Order{},skgo.Errorf(500,"mixed helper alias")};return Order{before,after,initializations,service.quantity},err
}
func failure(context.Context)(Order,error){return Order{},skgo.Errorf(418,"fixture-teapot")}
func coins(context.Context)(app.Money,error){return app.Money{Cents:price()},nil}
var _=skgo.Query(nested)
var _=skgo.Query(failure)
var _=skgo.Query(coins)
`

const contractHost = `package main
import("context";"encoding/json";"fmt";"io";"net/http";"net/http/httptest";"os";"runtime";"strings";"sync";"time";"github.com/tylergannon/skgo";"example.com/nativefixture/generated";orders "PACKAGE")
type candidate struct{Object string;Storage []string;Reclaim bool}
type command struct{Op,Method,Path,Body string;Headers map[string]string;Candidate candidate}
type response struct{Status,PID,Initializations,Consumers int;Body,Error,Diagnostic string;Headers http.Header}
type heldWriter struct{http.ResponseWriter;hold *orders.Hold;once sync.Once}
func(w *heldWriter)Write(p []byte)(int,error){w.once.Do(func(){close(w.hold.Entered);<-w.hold.Release});return w.ResponseWriter.Write(p)}
func(w *heldWriter)Flush(){if f,ok:=w.ResponseWriter.(http.Flusher);ok{f.Flush()}}
func main(){
 install:=func(candidate)error{return fmt.Errorf("production host")};usage:=func()(int,int){return 0,0}
 // HOT_INIT
 manifest,err:=skgo.ReadManifest(os.DirFS("web/build"));if err!=nil{panic(err)}
 var diagnostic string;var mu sync.Mutex
 panicReport:=func(value any,stack []byte){mu.Lock();defer mu.Unlock();diagnostic=fmt.Sprintf("%v\n%s",value,stack)}
 rc,lc,ec,hc:=manifest.RemoteConfig("http://localhost"),manifest.LoadConfig("http://localhost"),manifest.EndpointConfig("http://localhost"),manifest.HandleConfig()
 rc.Transport=generated.Transport();lc.Transport=generated.Transport()
 rc.OnPanic=func(_ string,value any,stack []byte){panicReport(value,stack)}
 lc.OnPanic=func(_ string,value any,stack []byte){panicReport(value,stack)}
 ec.OnPanic=func(_ string,_ string,value any,stack []byte){panicReport(value,stack)}
 remotes,err:=skgo.NewRemotes(rc,generated.Remotes()...);if err!=nil{panic(err)}
 loads,err:=skgo.NewLoads(lc,generated.Loads()...);if err!=nil{panic(err)}
 endpoints,err:=skgo.NewEndpoints(ec,generated.Endpoints()...);if err!=nil{panic(err)}
 actions,err:=skgo.NewActions(generated.Actions()...);if err!=nil{panic(err)}
 var handler http.Handler
 var hold *orders.Hold
 internal:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){handler.ServeHTTP(w,r)})
 renderer,err:=skgo.NewSSR(os.DirFS("web/build"),manifest,loads,remotes,skgo.SSROptions{Runtimes:2,Fetch:internal,Actions:actions,HandleFetch:func(ctx context.Context,r *http.Request,next skgo.Fetch)(*http.Response,error){if r.URL.Query().Has("renderHold")&&hold!=nil{close(hold.Entered);<-hold.Release};return next(r)}});if err!=nil{panic(err)}
 pages,err:=skgo.NewStaticHandler(os.DirFS("web/build"),skgo.WithSSR(renderer));if err!=nil{panic(err)}
 endpoints.SetErrorTemplate(renderer.ErrorTemplate());hc.Origin="http://localhost";hc.Loads=loads;hc.Static=skgo.ServedAsFile(pages)
 app:=generated.RequestBoundary(hc,loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
 fetchConfig:=skgo.FetchConfig{Origin:"http://localhost",Handler:internal}
 // HOT_FETCH
 stack:=fetchConfig.Intercept(app)
 handler=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){switch r.Header.Get("X-Hold"){case "execution":r=r.WithContext(context.WithValue(r.Context(),"hold",hold));case "serialization":w=&heldWriter{ResponseWriter:w,hold:hold}};stack.ServeHTTP(w,r)})
 // HOT_HANDLER
 streamEntered,streamRelease:=make(chan struct{}),make(chan struct{})
 host:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.URL.Path=="/host-stream"{w.Write([]byte("stream-open"));w.(http.Flusher).Flush();close(streamEntered);<-streamRelease;w.Write([]byte("stream-closed"));return};handler.ServeHTTP(w,r)}));defer host.Close()
 request:=func(c command)response{
  path:=c.Path;if strings.HasPrefix(path,"@"){name:=strings.TrimPrefix(path,"@");for _,remote:=range generated.Remotes(){if strings.HasSuffix(remote.ID(),"/"+name){path="/_app/remote/"+remote.ID();break}}}
  method:=c.Method;if method==""{method="GET"};req,_:=http.NewRequest(method,host.URL+path,strings.NewReader(c.Body));for k,v:=range c.Headers{req.Header.Set(k,v)}
  result,err:=http.DefaultClient.Do(req);if err!=nil{return response{Error:err.Error()}};raw,err:=io.ReadAll(result.Body);result.Body.Close();if err!=nil{return response{Error:err.Error()}};return response{Status:result.StatusCode,Body:string(raw),Headers:result.Header}
 }
 old:=make(chan response,1);streamDone:=make(chan response,1)
 decoder,encoder:=json.NewDecoder(os.Stdin),json.NewEncoder(os.Stdout)
 for{var c command;if err:=decoder.Decode(&c);err==io.EOF{return}else if err!=nil{panic(err)};runtime.GC();var result response
 switch c.Op{
 case "request":result=request(c)
 case "install":if err:=install(c.Candidate);err!=nil{result.Error=err.Error()}
 case "start":hold=&orders.Hold{Entered:make(chan struct{}),Release:make(chan struct{})};if c.Headers["X-Hold"]=="deferred"{orders.DeferredGate=hold};go func(){old<-request(c)}();select{case<-hold.Entered:case<-time.After(5*time.Second):result.Error="held consumer did not enter"}
 case "release":close(hold.Release);select{case result=<-old:case<-time.After(5*time.Second):result.Error="held consumer did not complete"};orders.DeferredGate=nil;hold=nil
 case "stream":go func(){streamDone<-request(command{Path:"/host-stream"})}();select{case<-streamEntered:case<-time.After(5*time.Second):result.Error="host stream did not enter"}
 case "endstream":close(streamRelease);select{case result=<-streamDone:case<-time.After(5*time.Second):result.Error="host stream did not complete"}
 case "diagnostics":mu.Lock();result.Diagnostic=diagnostic;mu.Unlock()
 default:result.Error="unknown operation"
 }
 result.PID=os.Getpid();observed,_:=orders.Skgo_order(context.Background());result.Initializations=observed.Initializations;_,result.Consumers=usage();if err:=encoder.Encode(result);err!=nil{panic(err)}
 }
}
`

func writeNativeRendererFixture(t *testing.T, app string, write func(string, string)) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(app, "web/skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest skgo.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.SkgoAdapter = adapter.Fingerprint()
	manifest.AppDir = "_app"
	manifest.Nodes = []string{"src/routes/orders/+page.server.ts"}
	manifest.Routes = []skgo.ManifestRoute{{ID: "/orders", Pattern: "^/orders/?$", Page: &skgo.ManifestPage{Leaf: 0}, Endpoint: &skgo.ManifestEndpoint{Methods: []string{"GET"}}}}
	manifest.SSR = &skgo.ManifestSSR{Bundle: "bundle.js", Template: "app.html", ErrorTemplate: "error.html", Target: "es2022", GlobalName: "__native_fixture", Nodes: []skgo.ManifestSSRNode{{Index: 0, Component: true}}}
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	write("web/build/skgo.manifest.json", string(raw))
	write("web/build/client/.keep", "")
	write("web/build/index.html", "<!doctype html><html><body>fallback</body></html>")
	write("web/build/app.html", "<!doctype html><html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>")
	write("web/build/error.html", "<!doctype html>%sveltekit.status% %sveltekit.error.message%")
	write("web/build/bundle.js", `globalThis.__skgo_ping=()=>"ok";
globalThis.__skgo_render=raw=>{
 const req=JSON.parse(raw),graph=JSON.parse(req.branch[0].data),order=graph[graph[0].order];
 const out={done:true,status:req.status,head:"",body:"<h1>Order total "+graph[order.total]+"</h1>"};
 if(req.url.includes("render=1")){
 out.done=false;__skgo_fetch(JSON.stringify({method:"GET",url:"http://localhost/orders?raw=1&renderHold=1",headers:{accept:"application/json"}})).then(raw=>{
 const answer=JSON.parse(raw);if(answer.error)throw Error(answer.error);out.body+="<p>Fetched "+answer.response.body+"</p>";out.done=true;
 },error=>{out.failure=String(error);out.done=true;});
 }return out;
};`)
}
