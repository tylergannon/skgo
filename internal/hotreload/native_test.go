//go:build skgo_qualification

package hotreload

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/generate"
	"golang.org/x/tools/go/packages"
)

// A separate unstripped executable contains the loader. Neither this normally
// stripped test driver nor the production SDK links it.
func TestGeneratedNativeLifetime(t *testing.T) {
	source := orderSource
	hostSource := nativeHost
	hostSource = strings.Replace(hostSource, `"context";`, "", 1)
	source = strings.Replace(source, "note string", "note string; callback func() int", 1)
	source = strings.Replace(source, `if hold,ok:=ctx.Value("hold").(*Hold);ok { close(hold.Entered);<-hold.Release };`, "", 1)
	source = strings.Replace(source, "func Box() any", "func Box() Order", 1)
	source = strings.Replace(source, "if err:=skgo.RefreshNoArg(ctx,order);err!=nil{return Order{},err};return order(ctx)", "return order(nil)", 1)
	hostSource = strings.Replace(hostSource, `;Reclaim bool}`, `;Reclaim bool};Ranges []nativeRange`, 1)
	hostSource = strings.Replace(hostSource, `case "check": result=check()`, `case "snapshot": result.Ranges=orders.SkgoHotRanges();result.Saved,result.Backing=orders.SkgoHotSaved()
 case "probe": result.Ranges=orders.SkgoHotProbe(command.Ranges);result.Saved,result.Backing=orders.SkgoHotSaved()
 case "check": result=check()`, 1)
	hostSource = strings.Replace(hostSource, `type observation struct{`, `type nativeRange = orders.SkgoNativeRange
 type observation struct{`, 1)
	hostSource = strings.Replace(hostSource, `Error string}`, `Error string;Ranges []nativeRange;Saved int;Backing uintptr}`, 1)
	hostSource = strings.Replace(hostSource, `func main(){`, `type heldWriter struct{http.ResponseWriter;hold *orders.Hold}
 func(w *heldWriter) Write(p []byte)(int,error){close(w.hold.Entered);<-w.hold.Release;return w.ResponseWriter.Write(p)}
 func main(){`, 1)
	hostSource = strings.Replace(hostSource, `withHold:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.Header.Get("X-Hold")=="yes" {r=r.WithContext(context.WithValue(r.Context(),"hold",hold))};registry.ServeHTTP(w,r)})`, `withHold:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.Header.Get("X-Hold")=="yes" {w=&heldWriter{ResponseWriter:w,hold:hold}};registry.ServeHTTP(w,r)})`, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Setenv("GOWORK", "")
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("CC", filepath.Join(t.TempDir(), "no-c-compiler"))
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	tc := sharedNativeToolchain(t, ctx)
	work, err = filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(work, "application")
	write := func(path string, raw []byte) {
		t.Helper()
		path = filepath.Join(app, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	mod, err := os.ReadFile(filepath.Join(root, "example/go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod = []byte(strings.ReplaceAll(strings.ReplaceAll(string(mod), "module github.com/tylergannon/skgo/example", "module example.com/nativefixture"), "=> ../", "=> "+filepath.ToSlash(root)))
	write("go.mod", mod)
	write("internal/app/locals.go", []byte("package app\ntype Locals struct{}\n"))
	write("web/package.json", []byte(`{"type":"module"}`))
	write("web/src/routes/orders/orders.remote.go", []byte(source))
	sums, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", sums)
	if raw, err := tc.command(ctx, app, "mod", "download").CombinedOutput(); err != nil {
		t.Fatalf("download private host dependencies: %v\n%s", err, raw)
	}
	kit := filepath.Join(app, "web/node_modules/@sveltejs")
	if err := os.MkdirAll(kit, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "example/web/node_modules/@sveltejs/kit"), filepath.Join(kit, "kit")); err != nil {
		t.Fatal(err)
	}
	prepared, err := generate.Prepare(generate.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/nativefixture/internal/app", LocalsType: "Locals", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err := prepared.Run(); err != nil {
		t.Fatal(err)
	}
	links, err := os.ReadDir(filepath.Join(app, "generated/_links"))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("fixture route packages: %v", links)
	}
	linkRel := filepath.Join("generated/_links", links[0].Name())
	linkDir := filepath.Join(app, linkRel)
	pkgPath := "example.com/nativefixture/" + filepath.ToSlash(linkRel)
	run := func(args ...string) []byte {
		t.Helper()
		raw, err := tc.command(ctx, app, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, raw)
		}
		return raw
	}
	run("mod", "tidy")
	load := func() *packages.Package {
		t.Helper()
		pkgs, err := packages.Load(&packages.Config{Context: ctx, Dir: app, Env: tc.command(ctx, app).Env, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}, pkgPath)
		if err != nil || len(pkgs) != 1 {
			t.Fatalf("load app: %v %v", pkgs, err)
		}
		if len(pkgs[0].Errors) != 0 {
			t.Fatal(pkgs[0].Errors)
		}
		return pkgs[0]
	}
	p := load()
	sources := map[string][]byte{}
	for _, file := range p.CompiledGoFiles {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources[file] = raw
	}
	lowered, err := Lower(p, sources)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range lowered.Host {
		write(filepath.Join(linkRel, name), raw)
	}
	write(filepath.Join(linkRel, "skgo_probe.go"), []byte(lifetimeProbe))
	write("cmd/main.go", []byte(strings.ReplaceAll(hostSource, "PACKAGE", pkgPath)))
	_, err = tc.PrepareLoader(ctx, app, filepath.Join(work, "native"))
	if err != nil {
		t.Fatal(err)
	}
	run("mod", "tidy")
	bin := filepath.Join(app, "host")
	run("build", "-ldflags=-s=false", "-o", bin, "./cmd")
	info, err := buildinfo.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	cgoDisabled := false
	for _, setting := range info.Settings {
		if setting.Key == "CGO_ENABLED" {
			cgoDisabled = setting.Value == "0"
		}
	}
	if !cgoDisabled {
		t.Fatal("native host was not built with CGO_ENABLED=0")
	}
	t.Logf("native host: %s %s/%s, CGO_ENABLED=0, no C compiler", info.GoVersion, tc.OS, tc.Arch)

	importcfg := run("list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", pkgPath)
	write("importcfg", importcfg)
	baseline := map[string][]byte{}
	for file, raw := range sources {
		baseline[file] = raw
	}
	baseline[filepath.Join(linkDir, "skgo_hot.go")] = []byte("package orders\n")
	baseline[filepath.Join(linkDir, "skgo_probe.go")] = []byte("package orders\n")
	captured := func(source string) map[string][]byte {
		next := map[string][]byte{}
		for file, raw := range baseline {
			next[file] = raw
		}
		next[filepath.Join(linkDir, "orders.remote.go")] = []byte(source)
		return next
	}
	analysis := &retainedApplication{files: baseline, analysis: &retainedAnalysis{}}
	t.Cleanup(analysis.analysis.close)
	compile := func(name, source string) (Candidate, error) {
		return tc.compileWithLoader(ctx, app, pkgPath, filepath.Join(app, "importcfg"), filepath.Join(app, name), baseline, captured(source), nil, analysis.Load)
	}
	candidate20, err := compile("discount20", strings.Replace(source, "return total", "return total * 80 / 100", 1))
	if err != nil {
		t.Fatal(err)
	}
	cmd := tc.command(ctx, app)
	cmd.Path = bin
	cmd.Args = []string{bin}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		in.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	decoder, encoder := json.NewDecoder(out), json.NewEncoder(in)
	type observation struct {
		Total, Again, Summary, Initializations, Quantity, PID int
		Identity, Refreshed                                   bool
		Error                                                 string
		Ranges                                                []nativeRange
		Saved                                                 int
		Backing                                               uintptr
	}
	var pid int
	invoke := func(op string, candidate Candidate, ranges []nativeRange) observation {
		t.Helper()
		if err := encoder.Encode(struct {
			Op        string
			Candidate Candidate
			Ranges    []nativeRange
		}{op, candidate, ranges}); err != nil {
			t.Fatal(err)
		}
		var result observation
		if err := decoder.Decode(&result); err != nil {
			t.Fatalf("%s: %v\n%s", op, err, stderr.String())
		}
		if op != "probe" {
			t.Logf("%s: %+v", op, result)
		}
		return result
	}
	call := func(op string, candidate Candidate) observation { return invoke(op, candidate, nil) }
	check := func(op string, want int) observation {
		t.Helper()
		result := call(op, Candidate{})
		if pid == 0 {
			pid = result.PID
		}
		if result.Error != "" || result.Total != want || result.Again != want || (op != "release" && result.Summary != want) || result.Initializations != 1 || result.Quantity != 3 || result.PID != pid || !result.Identity {
			t.Fatalf("%s: %+v, want coherent %d, quantity 3, init 1, stable PID/types", op, result, want)
		}
		return result
	}
	if !candidate20.Reclaim {
		t.Fatal("numeric helper did not qualify")
	}
	check("mutate", 4200)
	// Missing storage must trigger host replacement before touching the object.
	// This fabricated candidate needs no additional compiler invocation.
	refused := call("install", Candidate{Object: "does-not-exist.o", Storage: []string{"fixture.missing.host.storage"}})
	if !strings.Contains(refused.Error, "missing host storage: fixture.missing.host.storage") {
		t.Fatalf("storage checked after object access: %+v", refused)
	}
	check("check", 4200)
	candidate10, err := compile("discount10", strings.Replace(source, "return total", "return total * 90 / 100", 1))
	if err != nil {
		t.Fatal(err)
	}
	if !candidate10.Reclaim {
		t.Fatal("numeric helper did not qualify")
	}
	if result := call("install", candidate10); result.Error != "" {
		t.Fatal(result.Error)
	}
	check("check", 3780)
	ranges := call("snapshot", Candidate{}).Ranges
	if len(ranges) != 2 {
		t.Fatalf("expected text/data ranges: %+v", ranges)
	}
	for _, r := range ranges {
		if !r.Mapped || r.Base == 0 || r.Size == 0 {
			t.Fatalf("missing native range: %+v", r)
		}
		if tc.OS == "darwin" && r.Object == 0 {
			t.Fatal("missing native kernel object identity")
		}
	}
	call("hold", Candidate{})
	if result := call("install", candidate20); result.Error != "" {
		t.Fatal(result.Error)
	}
	check("check", 3360)
	held := invoke("probe", Candidate{}, ranges).Ranges
	if len(held) != 2 {
		t.Fatalf("missing held mapping observations: %+v", held)
	}
	for _, r := range held {
		if !r.Mapped {
			t.Fatalf("unmapped while response held: %+v", r)
		}
	}
	check("release", 3780)
	var released []nativeRange
	deadline := time.Now().Add(5 * time.Second)
	for {
		released = invoke("probe", Candidate{}, ranges).Ranges
		mapped := false
		for _, r := range released {
			mapped = mapped || r.Mapped
		}
		if !mapped || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(released) != 2 {
		t.Fatalf("missing retirement observations: %+v", released)
	}
	for _, r := range released {
		if r.Mapped {
			t.Fatalf("retired native mapping remains: %+v\n%s", r, stderr.String())
		}
	}
	t.Logf("released native ranges: %+v", released)
	retained, err := compile("retained", strings.Replace(source, "return total", `service.callback=func()int{return 71};return total * 70 / 100`, 1))
	if err != nil {
		t.Fatal(err)
	}
	if retained.Reclaim {
		t.Fatal("escaping closure incorrectly reclaimable")
	}
	if result := call("install", retained); result.Error != "" {
		t.Fatal(result.Error)
	}
	check("check", 2940)
	backing := call("snapshot", Candidate{})
	if backing.Saved != 71 || backing.Backing == 0 {
		t.Fatalf("candidate closure not installed: %+v", backing)
	}
	backed := false
	for _, r := range backing.Ranges {
		if backing.Backing >= r.Base && backing.Backing < r.Base+r.Size && r.Mapped {
			backed = true
		}
	}
	if !backed {
		t.Fatalf("saved closure does not point into candidate mappings: %+v", backing)
	}
	if result := call("install", candidate20); result.Error != "" {
		t.Fatal(result.Error)
	}
	check("check", 3360)
	survived := invoke("probe", Candidate{}, backing.Ranges)
	if survived.Saved != 71 || survived.Backing != backing.Backing || len(survived.Ranges) != 2 {
		t.Fatalf("retained closure lost after swaps/GC: %+v", survived)
	}
	for _, r := range survived.Ranges {
		if !r.Mapped {
			t.Fatalf("retained backing unmapped: %+v", r)
		}
	}
	t.Logf("retained closure after swap/GC: value=%d address=%x ranges=%+v", survived.Saved, survived.Backing, survived.Ranges)
	for _, candidate := range []Candidate{candidate10, {Object: "does-not-exist.o"}} {
		if result := call("install", candidate); !strings.Contains(result.Error, "retained revision budget exhausted") {
			t.Fatalf("retained budget refusal: %+v", result)
		}
	}
	check("check", 3360)
	call("finish", Candidate{})
	in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("generated lifetime host: %v\n%s", err, stderr.String())
	}
}

const orderSource = `package orders
import("context";"github.com/tylergannon/skgo";"example.com/nativefixture/generated/params")
type Order struct { Total int ` + "`json:\"total\"`" + `; Again int ` + "`json:\"again\"`" + `; Initializations int ` + "`json:\"initializations\"`" + `; Quantity int ` + "`json:\"quantity\"`" + ` }
func (Order) Label() string { return "order" }
type Hold struct { Entered, Release chan struct{} }
type serviceState struct { quantity int; note string }
var service = &serviceState{quantity:2}
var initializations int
var unusedStorage int
func init(){ initializations++ }
func price() int { total:=service.quantity*1200+600;return total }
func order(ctx context.Context)(Order,error){ before:=price();if hold,ok:=ctx.Value("hold").(*Hold);ok { close(hold.Entered);<-hold.Release };return Order{before,price(),initializations,service.quantity},nil }
func summary(context.Context)(int,error){ return price(),nil }
func Box() any {return Order{price(),price(),initializations,service.quantity}}
func quantity(ctx context.Context,event params.RequestEvent,n int)(Order,error){service.quantity=n;if err:=skgo.RefreshNoArg(ctx,order);err!=nil{return Order{},err};return order(ctx)}
var _=skgo.Query(order)
var _=skgo.Query(summary)
var _=skgo.Command(quantity)
`

const nativeHost = `package main
import("context";"encoding/json";"fmt";"io";"net/http";"net/http/httptest";"os";"reflect";"runtime";"strings";"time";"github.com/tylergannon/devalue/v5";"github.com/tylergannon/skgo";"example.com/nativefixture/generated";orders "PACKAGE")
func main(){
 if err:=orders.SkgoHotInit(2);err!=nil{panic(err)}
 remotes:=generated.Remotes();registry,err:=skgo.NewRemotes(skgo.RemoteConfig{Origin:"http://localhost"},remotes...);if err!=nil{panic(err)}
 var orderRemote,summaryRemote,quantityRemote *skgo.Remote;for _,r:=range remotes {if strings.HasSuffix(r.ID(),"/order"){orderRemote=r};if strings.HasSuffix(r.ID(),"/summary"){summaryRemote=r};if strings.HasSuffix(r.ID(),"/quantity"){quantityRemote=r}}
 if orderRemote==nil||summaryRemote==nil||quantityRemote==nil{panic("missing generated remotes")}
 var hold *orders.Hold
 withHold:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.Header.Get("X-Hold")=="yes" {r=r.WithContext(context.WithValue(r.Context(),"hold",hold))};registry.ServeHTTP(w,r)})
 host:=httptest.NewServer(orders.SkgoHotHandler(withHold));defer host.Close()
 type observation struct{Total,Again,Summary,Initializations,Quantity,PID int;Identity,Refreshed bool;Error string}
 original:=reflect.ValueOf(orders.Skgo_order).Pointer()
 fetch:=func(remote *skgo.Remote,held bool) any {
  req,_:=http.NewRequest("GET",host.URL+"/_app/remote/"+remote.ID(),nil);if held{req.Header.Set("X-Hold","yes")}
  response,err:=http.DefaultClient.Do(req);if err!=nil{panic(err)}
  raw,err:=io.ReadAll(response.Body);response.Body.Close();if err!=nil{panic(err)}
  var envelope struct{Type,Data string};if err:=json.Unmarshal(raw,&envelope);err!=nil{panic(err)}
  if response.StatusCode!=200||envelope.Type!="result"{panic(string(raw))}
  tree,err:=devalue.Parse(envelope.Data,nil);if err!=nil{panic(err)};object,ok:=tree.(*devalue.Object);if !ok{panic(string(raw))};value,ok:=object.Get("_");if !ok{panic(string(raw))};return value
 }
 observe:=func(value any) observation {
  order,err:=generated.DecodeOrder(value);if err!=nil{panic(err)}
  return observation{Total:order.Total,Again:order.Again,Initializations:order.Initializations,Quantity:order.Quantity,PID:os.Getpid(),Identity:reflect.ValueOf(orders.Skgo_order).Pointer()==original}
 }
 check:=func() observation {
  runtime.GC()
  result:=observe(fetch(orderRemote,false))
  summary,ok:=fetch(summaryRemote,false).(float64);if !ok{panic("invalid summary wire value")};result.Summary=int(summary)
  boxed:=any(orders.Box());typed,same:=boxed.(orders.Order);result.Identity=result.Identity&&same&&typed.Total==result.Total&&typed.Again==result.Again
  return result
 }
 old:=make(chan observation,1)
 decoder,encoder:=json.NewDecoder(os.Stdin),json.NewEncoder(os.Stdout)
 for {
  var command struct{Op string;Candidate struct{Object string;Storage []string;Reclaim bool}}
  if err:=decoder.Decode(&command);err!=nil{panic(err)}
  var result observation
  switch command.Op {
  case "mutate":
   req,_:=http.NewRequest("POST",host.URL+"/_app/remote/"+quantityRemote.ID(),strings.NewReader("{\"payload\":\"WzNd\"}"));req.Header.Set("Content-Type","application/json");req.Header.Set("Origin","http://localhost")
   res,err:=http.DefaultClient.Do(req);if err!=nil{panic(err)};raw,_:=io.ReadAll(res.Body);res.Body.Close();if res.StatusCode!=200{panic(string(raw))};var envelope struct{Type,Data string};if err:=json.Unmarshal(raw,&envelope);err!=nil||envelope.Type!="result"{panic(string(raw))}
   result=check();result.Refreshed=strings.Contains(envelope.Data,orderRemote.ID()+"/")
  case "hold":
   hold=&orders.Hold{Entered:make(chan struct{}),Release:make(chan struct{})}
   go func(){old<-observe(fetch(orderRemote,true))}()
   select{case <-hold.Entered:case <-time.After(5*time.Second):panic("old request did not enter")}
  case "release":
   close(hold.Release)
   select{case result=<-old:case <-time.After(5*time.Second):panic("old request did not complete")}
  case "install":
   if err:=orders.SkgoHotInstall(command.Candidate.Object,command.Candidate.Storage,command.Candidate.Reclaim);err!=nil{result.Error=err.Error()}
  case "check": result=check()
  case "finish": if err:=encoder.Encode(result);err!=nil{panic(err)};return
  default: panic(fmt.Sprint("unknown operation: ",command.Op))
  }
  if err:=encoder.Encode(result);err!=nil{panic(err)}
 }
}
`
