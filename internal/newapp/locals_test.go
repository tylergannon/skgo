package newapp

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Build a custom-module starter from the actual shipped templates. One frontend
// build supplies every served assertion, including both internal-fetch entries.
func TestStarterTypedLocalsThroughBothFetchPaths(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := project{Dir: dir, Module: "example.com/custom-locals", BindingsImport: "example.com/custom-locals/internal/skgo", GoVersion: "1.27.1", SkgoVersion: "v0.0.0", SkgoReplace: repo, App: "locals", Origin: "http://localhost:8080"}
	if err := writeGoFiles(p); err != nil {
		t.Fatal(err)
	}
	write := func(name, source string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"package.json", "tsconfig.json", "src/app.html"} {
		source, err := os.ReadFile(filepath.Join(repo, "example/web", file))
		if err != nil {
			t.Fatal(err)
		}
		write("web/"+file, string(source))
	}
	// Kit writes app-specific $app types during sync/build. Share installed
	// packages, while keeping generated types and Vite's temp files local.
	dependencies := filepath.Join(repo, "example/web/node_modules")
	target := filepath.Join(dir, "web/node_modules")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "$app" || entry.Name() == ".vite-temp" {
			continue
		}
		if err := os.Symlink(filepath.Join(dependencies, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
			t.Fatal(err)
		}
	}

	write("web/vite.config.ts", `import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import skgo from '@skgo/sveltekit-adapter';
export default defineConfig({plugins:[sveltekit({adapter:skgo(),paths:{origin:'http://localhost:8080'},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}})]});`)
	write("domain/principal.go", "package domain\ntype Principal struct{Name string}\n")
	locals := `package app
import("github.com/tylergannon/skgo";"example.com/custom-locals/domain")
type Locals struct{Principal domain.Principal}
type RequestEvent[P any] = skgo.RequestEvent[P,Locals]
`
	write("internal/app/locals.go", locals)
	hooks := `package serverhooks
import("context";"net/http";"github.com/tylergannon/skgo";"example.com/custom-locals/domain";"example.com/custom-locals/internal/app";"example.com/custom-locals/internal/skgo/params")
var Handle params.Middleware = func(ctx context.Context,event params.RequestEvent,resolve params.Resolve)(*http.Response,error){
 name:="parent-selected";if event.IsSubRequest(){name="subrequest-selected"}
 event.Locals=&app.Locals{Principal:domain.Principal{Name:name}}
 return resolve(ctx,event,skgo.ResolveOptions{})
}
`
	write("internal/serverhooks/handle.go", hooks)
	write("web/src/routes/page.server.go", `package routes
import("context";"io";"net/http";"github.com/tylergannon/skgo";"example.com/custom-locals/internal/app")
type Data struct{Name string `+"`json:\"name\"`"+`;GoFetch string `+"`json:\"goFetch\"`"+`;Nested string `+"`json:\"nested\"`"+`}
func pageLoad(event PageRequestEvent)(Data,error){
 ctx:=event.Context();if app.LocalsFrom(ctx)!=event.Locals{panic("typed/context mismatch")}
 request,_:=http.NewRequest("GET","/api/locals",nil);response,err:=event.Fetch(ctx,request);if err!=nil{return Data{},err};defer response.Body.Close();body,err:=io.ReadAll(response.Body);if err!=nil{return Data{},err}
 value,err:=nested(ctx);return Data{Name:event.Locals.Principal.Name,GoFetch:string(body),Nested:value},err
}
var _=skgo.Load(pageLoad)
func save(ctx context.Context)(Data,error){app.LocalsFrom(ctx).Principal.Name="action-selected";return Data{Name:"action-result"},nil}
var _=skgo.DefaultAction(save)
`)
	write("web/src/routes/locals.remote.go", `package routes
import("context";"github.com/tylergannon/skgo";"example.com/custom-locals/internal/app";"example.com/custom-locals/internal/skgo/params")
func nested(ctx context.Context)(string,error){return app.LocalsFrom(ctx).Principal.Name,nil}
var _=skgo.Query(nested)
func change(ctx context.Context,event params.RequestEvent)(string,error){
 if app.LocalsFrom(ctx)!=event.Locals{panic("command context binding mismatch")}
 value,err:=nested(ctx);if err!=nil{return "",err}
 if err:=skgo.RefreshRequestedNoArg(ctx,nested);err!=nil{return "",err}
 return "command:"+value,nil
}
var _=skgo.Command(change)
type Input struct {Note string `+"`json:\"note\"`"+`}
func submit(ctx context.Context,event params.RequestEvent,input Input)(string,error){if app.LocalsFrom(ctx)!=event.Locals{panic("form context mismatch")};return "form:"+event.Locals.Principal.Name+":"+input.Note,nil}
var _=skgo.Form(submit)
`)
	write("web/src/routes/layout.server.go", `package routes
import "github.com/tylergannon/skgo"
type Layout struct{LayoutName string `+"`json:\"layoutName\"`"+`}
func layoutLoad(event LayoutRequestEvent)(Layout,error){return Layout{LayoutName:event.Locals.Principal.Name},nil}
var _=skgo.Load(layoutLoad)
`)
	write("web/src/routes/api/locals/server.go", `package locals
import("fmt";"net/http";"github.com/tylergannon/skgo";"example.com/custom-locals/internal/app")
func get(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,app.LocalsFrom(r.Context()).Principal.Name)}
var _=skgo.GET(get)
`)
	write("web/src/routes/+page.ts", `export const load=async ({fetch,data})=>({...data,rendererFetch:await (await fetch('/api/locals')).text()});`)
	write("web/src/routes/+page.svelte", `<script lang="ts">import {change,submit,nested} from "./locals.remote";let {data,form}=$props();</script><p id="locals">{data.name}|{data.layoutName}|{data.nested}|{data.goFetch}|{data.rendererFetch}</p>{#if form}<p>{form.name}</p>{/if}<button onclick={()=>change()}>Command</button><button onclick={()=>nested().refresh()}>Refresh</button><form {...submit}><input name="note"/><button>Submit</button></form>`)
	write("server_test.go", starterLocalsHandlerTest)
	run := func(work string, args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = filepath.Join(dir, work)
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOMAXPROCS=2")
		out, err := cmd.CombinedOutput()
		if err != nil || bytes.Contains(out, []byte("--- SKIP")) {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		t.Logf("%v exited 0", args)
	}
	// Tidy after an empty first generation? Generated hook aliases must exist
	// before tidy traverses imports. The generator's ordinary dependency loader
	// populates sums when loading the explicit domain package.
	sums, err := os.ReadFile(filepath.Join(repo, "example/go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(sums))
	run("", "go", "get", "-tool", "github.com/tylergannon/skgo/cmd/skgo@v0.0.0")
	run("", "go", "generate", "./...")
	run("", "go", "mod", "tidy")
	run("", "go", "generate", "./...")
	for _, file := range []struct{ path, source string }{{"internal/app/locals.go", locals}, {"internal/serverhooks/handle.go", hooks}} {
		got, err := os.ReadFile(filepath.Join(dir, file.path))
		if err != nil || string(got) != file.source {
			t.Fatalf("authored %s changed", file.path)
		}
	}
	run("web", filepath.Join(repo, "example/web/node_modules/.bin/vp"), "build")
	run("", "go", "test", "-count=1", "-v", "./...")
	// The template and module-specific directive are part of the observed result.
	cfg, err := os.ReadFile(filepath.Join(dir, "internal/skgo/config.go"))
	if err != nil || !strings.Contains(string(cfg), "--locals-package example.com/custom-locals/internal/app") {
		t.Fatal("custom locals selection missing")
	}
}

const starterLocalsHandlerTest = `package app_test
import("strconv";"io/fs";"bytes";"encoding/binary";"encoding/json";"net/http/httptest";"strings";"testing";app "example.com/custom-locals";"example.com/custom-locals/web";generated "example.com/custom-locals/internal/skgo";hooks "example.com/custom-locals/internal/serverhooks";"github.com/tylergannon/polytype/devalue")
func TestServedStarterLocals(t *testing.T){
 dist,err:=fs.Sub(web.Build,"build");if err!=nil{t.Fatal(err)}
 handler,_,err:=app.NewHandler(dist,"","http://localhost:8080");if err!=nil{t.Fatal(err)}
 for _,tc:=range []struct{method,body,want string}{
 {"GET","","parent-selected|parent-selected|parent-selected|subrequest-selected|subrequest-selected"},
 {"POST","note=saved","action-selected|action-selected|action-selected|subrequest-selected|subrequest-selected"},
 }{
 request:=httptest.NewRequest(tc.method,"http://localhost:8080/",strings.NewReader(tc.body));if tc.method=="POST"{request.Header.Set("Content-Type","application/x-www-form-urlencoded");request.Header.Set("Origin","http://localhost:8080")}
 request.Header.Set("Accept","text/html");rec:=httptest.NewRecorder();handler.ServeHTTP(rec,request)
 if rec.Code!=200 || !strings.Contains(strings.ReplaceAll(rec.Body.String(),"<!---->",""),tc.want){t.Fatalf("%s: %d %s",tc.method,rec.Code,rec.Body.String())}
 if tc.method=="POST" && !strings.Contains(rec.Body.String(),"action-result"){t.Fatal("action response missing")}
 }
 remotes:=map[string]string{};for _,r:=range generated.Remotes(){remotes[r.Name()]=r.ID()}
 for _,name:=range []string{"change","submit"} {
  var body []byte
  media:="application/json"
  if name=="change" {body=[]byte("{\"payload\":\"\",\"refreshes\":["+strconv.Quote(remotes["nested"]+"/")+"]}")}else{
   header:="[[1,3],{\"note\":2},\"literal-note\",{}]";body=make([]byte,7+len(header));binary.LittleEndian.PutUint32(body[1:5],uint32(len(header)));copy(body[7:],header);media="application/x-sveltekit-formdata"
  }
  request:=httptest.NewRequest("POST","http://localhost:8080/_app/remote/"+remotes[name],bytes.NewReader(body));request.Header.Set("Content-Type",media);request.Header.Set("Origin","http://localhost:8080");request.Header.Set("x-sveltekit-pathname","/")
  rec:=httptest.NewRecorder();handler.ServeHTTP(rec,request)
  var env struct{Type,Data string};if err:=json.Unmarshal(rec.Body.Bytes(),&env);err!=nil{t.Fatal(err)}
  if rec.Code!=200 || env.Type!="result"{t.Fatalf("%s: %d %s",name,rec.Code,rec.Body.String())}
  tree,err:=devalue.Parse(env.Data,nil);if err!=nil{t.Fatal(err)};root:=tree.(*devalue.Object);value,_:=root.Get("_")
  want:="command:parent-selected"
  if name=="submit"{value,_=value.(*devalue.Object).Get("result");want="form:parent-selected:literal-note"}
  if value!=want{t.Fatalf("%s locals = %v want %s",name,value,want)}
  if name=="change" {updates,ok:=root.Get("q");if !ok{t.Fatalf("missing refreshed query: %s",env.Data)};entry,ok:=updates.(*devalue.Object).Get(remotes["nested"]+"/");if !ok{t.Fatalf("missing refresh key: %s",env.Data)};result,ok:=entry.(*devalue.Object).Get("v");if !ok || result!="parent-selected"{t.Fatalf("refreshed query = %v",entry)}}
 }
}

func TestServedStarterNoHook(t *testing.T){
 previous:=hooks.Handle;hooks.Handle=nil;defer func(){hooks.Handle=previous}()
 dist,err:=fs.Sub(web.Build,"build");if err!=nil{t.Fatal(err)}
 handler,_,err:=app.NewHandler(dist,"","http://localhost:8080");if err!=nil{t.Fatal(err)}
 for range 2 {
  rec:=httptest.NewRecorder();handler.ServeHTTP(rec,httptest.NewRequest("GET","http://localhost:8080/",nil))
  if rec.Code!=200 || !strings.Contains(strings.ReplaceAll(rec.Body.String(),"<!---->",""),"<p id=\"locals\">||||</p>"){t.Fatalf("no-hook stack: %d %s",rec.Code,rec.Body.String())}
 }
}
`
