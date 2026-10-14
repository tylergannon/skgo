package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
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
func TestGeneratedNativeState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Setenv("GOWORK", "off")
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	tc, err := PrepareToolchain(ctx, "go", filepath.Join(work, "toolchain"))
	if err != nil {
		t.Fatal(err)
	}
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
	write("web/src/routes/orders/orders.remote.go", []byte(orderSource))
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
	links, err := os.ReadDir(filepath.Join(app, "generated/links"))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("fixture route packages: %v", links)
	}
	linkRel := filepath.Join("generated/links", links[0].Name())
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
	load := func(overlay map[string][]byte) *packages.Package {
		t.Helper()
		pkgs, err := packages.Load(&packages.Config{Context: ctx, Dir: app, Env: tc.command(ctx, app).Env, Overlay: overlay, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}, pkgPath)
		if err != nil || len(pkgs) != 1 {
			t.Fatalf("load app: %v %v", pkgs, err)
		}
		if len(pkgs[0].Errors) != 0 {
			t.Fatal(pkgs[0].Errors)
		}
		return pkgs[0]
	}
	p := load(nil)
	sources := map[string][]byte{}
	for _, file := range p.CompiledGoFiles {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sources[file] = raw
	}
	t.Run("AliasRetention", func(t *testing.T) {
		changed := map[string][]byte{}
		for path, raw := range sources {
			changed[path] = raw
		}
		file := filepath.Join(linkDir, "orders.remote.go")
		changed[file] = []byte(strings.Replace(orderSource, "service.quantity=n", `alias:=service;alias.note="retained";alias.quantity=n`, 1))
		next := load(changed)
		err := Classify(sources, changed, []*packages.Package{next}, []*packages.Package{next})
		if err != nil {
			t.Errorf("alias-retention source is no longer classified by the serial compatibility scaffold: %v", err)
		}
	})
	for _, fixture := range []struct{ name, body string }{
		{"PrimitiveAlias", `alias:=service;alias.quantity=n`},
		{"LocalString", `var local struct{ note string };local.note="transient";_ = local;service.quantity=n`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			changed := map[string][]byte{}
			for path, raw := range sources {
				changed[path] = raw
			}
			changed[filepath.Join(linkDir, "orders.remote.go")] = []byte(strings.Replace(orderSource, "service.quantity=n", fixture.body, 1))
			next := load(changed)
			if err := Classify(sources, changed, []*packages.Package{next}, []*packages.Package{next}); err != nil {
				t.Errorf("ordinary synchronous body rejected: %v", err)
			}
		})
	}
	lowered, err := Lower(p, sources)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range lowered.Host {
		write(filepath.Join(linkRel, name), raw)
	}
	write("cmd/main.go", []byte(strings.ReplaceAll(nativeHost, "PACKAGE", pkgPath)))
	run("mod", "edit", "-require=github.com/pkujhd/goloader@"+LoaderVersion)
	run("mod", "tidy")
	bin := filepath.Join(app, "host")
	run("build", "-ldflags=-s=false", "-o", bin, "./cmd")
	importcfg := run("list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", pkgPath)
	write("importcfg", importcfg)
	var objects []string
	for revision, discount := range []int{0, 10, 20} {
		captured := map[string][]byte{}
		for file, raw := range sources {
			captured[file] = raw
		}
		file := filepath.Join(linkDir, "orders.remote.go")
		captured[file] = []byte(strings.Replace(orderSource, "return total", fmt.Sprintf("return total * %d / 100", 100-discount), 1))
		captured[filepath.Join(linkDir, "skgo_hot.go")] = []byte("package orders\n")
		// Type-check the captured authored revision, not the transformed host.
		p := load(captured)
		baseline := map[string][]byte{}
		for file, raw := range sources {
			baseline[file] = raw
		}
		baseline[filepath.Join(linkDir, "skgo_hot.go")] = []byte("package orders\n")
		if err := Classify(baseline, captured, []*packages.Package{p}, []*packages.Package{p}); err != nil {
			t.Fatalf("pricing revision admission: %v", err)
		}
		next, err := Lower(p, captured)
		if err != nil {
			t.Fatal(err)
		}
		var files []string
		for name, raw := range next.Candidate {
			rel := filepath.Join(fmt.Sprintf("candidate%d", revision), name)
			write(rel, raw)
			files = append(files, filepath.Join(app, rel))
		}
		object := filepath.Join(app, fmt.Sprintf("candidate%d.o", revision))
		args := append([]string{"tool", "compile", "-p", pkgPath, "-lang=go1.27", "-importcfg", filepath.Join(app, "importcfg"), "-o", object}, files...)
		run(args...)
		objects = append(objects, object)
	}
	cmd := tc.command(ctx, app)
	cmd.Path = bin
	cmd.Args = append([]string{bin}, objects...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated host: %v\n%s", err, output)
	}
	t.Logf("%s", output)
	var results []struct {
		Total, Initializations, Quantity int
		Identity                         bool
	}
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("want four actual HTTP observations: %s", output)
	}
	for i, total := range []int{3000, 2700, 2400, 3360} {
		quantity := 2
		if i == 3 {
			quantity = 3
		}
		if results[i].Total != total || results[i].Quantity != quantity || results[i].Initializations != 1 || !results[i].Identity {
			t.Fatalf("revision %d: %s, want total %d, initializer 1 and host type identity", i, output, total)
		}
	}
}

const orderSource = `package orders
import("context";"github.com/tylergannon/skgo";"example.com/nativefixture/generated/params")
type Order struct { Total int ` + "`json:\"total\"`" + `; Initializations int ` + "`json:\"initializations\"`" + `; Quantity int ` + "`json:\"quantity\"`" + ` }
type serviceState struct { quantity int; note string }
var service = &serviceState{quantity:2}
var initializations int
func init(){ initializations++ }
func price() int { total:=service.quantity*1200+600;return total }
func order(context.Context)(Order,error){ return Order{price(),initializations,service.quantity},nil }
func summary(context.Context)(int,error){ return price(),nil }
func Box() any {return Order{price(),initializations,service.quantity}}
func quantity(ctx context.Context,event params.RequestEvent,n int)(Order,error){service.quantity=n;if err:=skgo.RefreshNoArg(ctx,order);err!=nil{return Order{},err};return order(ctx)}
var _=skgo.Query(order)
var _=skgo.Query(summary)
var _=skgo.Command(quantity)
`

const nativeHost = `package main
import("encoding/json";"fmt";"io";"net/http";"net/http/httptest";"os";"reflect";"runtime";"strings";"github.com/pkujhd/goloader";"github.com/tylergannon/devalue/v5";"github.com/tylergannon/skgo";"example.com/nativefixture/generated";orders "PACKAGE")
func main(){
 syms:=map[string]uintptr{};if err:=goloader.RegSymbol(syms);err!=nil{panic(err)}
 goloader.RegTypes(syms,orders.SkgoHotTypes()...)
 remotes:=generated.Remotes();registry,err:=skgo.NewRemotes(skgo.RemoteConfig{Origin:"http://localhost"},remotes...);if err!=nil{panic(err)}
 var orderRemote,quantityRemote *skgo.Remote;for _,r:=range remotes {if strings.HasSuffix(r.ID(),"/order"){orderRemote=r};if strings.HasSuffix(r.ID(),"/quantity"){quantityRemote=r}}
 if orderRemote==nil||quantityRemote==nil{panic("missing generated remotes")}
 host:=httptest.NewServer(registry);defer host.Close()
 type result struct{Total,Initializations,Quantity int;Identity bool}
 var results []result
 // Keep every loaded mapping alive until this short-lived host exits. This
 // serial fixture does not claim that replacement mappings can be retired.
 original:=reflect.ValueOf(orders.Skgo_order).Pointer()
 check:=func(){
  response,err:=http.Get(host.URL+"/_app/remote/"+orderRemote.ID());if err!=nil{panic(err)}
  raw,err:=io.ReadAll(response.Body);response.Body.Close();if err!=nil{panic(err)}
  var envelope struct{Type,Data string};if err:=json.Unmarshal(raw,&envelope);err!=nil{panic(err)}
  if envelope.Type!="result"{panic(string(raw))}
  // Decode actual HTTP bytes; expected values are independent literals in
  // the test driver. A separate loaded interface result must assert to the
  // host's existing named application type, not a second type descriptor.
  tree,err:=devalue.Parse(envelope.Data,nil);if err!=nil{panic(err)};object,ok:=tree.(*devalue.Object);if !ok{panic(string(raw))};orderValue,ok:=object.Get("_");if !ok{panic(string(raw))};value,err:=generated.DecodeOrder(orderValue);if err!=nil{panic(err)}
  boxed:=orders.Box();typed,same:=boxed.(orders.Order);same=same&&typed.Total==value.Total
  results=append(results,result{value.Total,value.Initializations,value.Quantity,same&&reflect.ValueOf(orders.Skgo_order).Pointer()==original})
 }
 for _,object:=range os.Args[1:] {
  linker,err:=goloader.ReadObjs([]string{object},[]string{"PACKAGE"});if err!=nil{panic(err)}
  if missing:=goloader.UnresolvedSymbols(linker,syms);len(missing)>0{panic(fmt.Sprint(missing))}
  runtime.LockOSThread();next,err:=goloader.Load(linker,syms);runtime.UnlockOSThread();if err!=nil{panic(err)}
  install,err:=orders.SkgoHotPrepare(next.Syms);if err!=nil{panic(err)};install()
  runtime.GC();check()
 }
 // Mutate the authored package state after several swaps, then read it through
 // both the generated command and query, including named-query refresh.
 req,_:=http.NewRequest("POST",host.URL+"/_app/remote/"+quantityRemote.ID(),strings.NewReader("{\"payload\":\"WzNd\"}"));req.Header.Set("Content-Type","application/json");req.Header.Set("Origin","http://localhost")
 res,err:=http.DefaultClient.Do(req);if err!=nil{panic(err)};raw,_:=io.ReadAll(res.Body);res.Body.Close();if res.StatusCode!=200{panic(string(raw))};var command struct{Type,Data string};if err:=json.Unmarshal(raw,&command);err!=nil||command.Type!="result"{panic(string(raw))};if !strings.Contains(command.Data,orderRemote.ID()+"/"){panic("missing named-query refresh: "+string(raw))};check()
 if err:=json.NewEncoder(os.Stdout).Encode(results);err!=nil{panic(err)}
}
`
