package adapter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionRouterDocumentsUseNativeKitShell(t *testing.T) {
	for _, mode := range []string{"hash", "pathname"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			fixture := filepath.Join(t.TempDir(), "application")
			if retained := os.Getenv("SKGO_HASH_FIXTURE_DIR"); retained != "" {
				fixture = filepath.Join(retained, mode)
			}
			if err := stageMinimalInputsBootstrap(fixture); err != nil {
				t.Fatal(err)
			}
			web := filepath.Join(fixture, "web")
			writeFixtureFile(t, filepath.Join(fixture, "internal", "skgo", "config.go"), "package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app\n")
			writeFixtureFile(t, filepath.Join(fixture, "internal", "app", "locals.go"), "package app\ntype Locals struct{}\n")
			writeFixtureFile(t, filepath.Join(web, "src", "routes", "+page.svelte"), `<h1>HASH_ROUTER_HOME_42</h1><a href="#/about">About</a>`)
			writeFixtureFile(t, filepath.Join(web, "src", "routes", "about", "+page.svelte"), `<h1>HASH_ROUTER_ABOUT_42</h1><a href="#/">Home</a>`)
			writeFixtureFile(t, filepath.Join(web, "static", "asset-245.txt"), "HASH_ASSET_245\n")
			template := filepath.Join(web, "src", "app.html")
			source, err := os.ReadFile(template)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, template, strings.Replace(string(source), "<head>", "<head><meta name=\"fixture\" content=\"HASH_ROUTER_SHELL_42\">", 1))
			configPath := filepath.Join(web, "vite.config.ts")
			config, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			updated := strings.Replace(string(config), "adapter: skgo({ precompress: false }),", "adapter: skgo({ precompress: false }),\n router: { type: '"+mode+"' },", 1)
			updated = strings.ReplaceAll(updated, "http://127.0.0.1:8080", "http://127.0.0.1:4185")
			writeFixtureFile(t, configPath, updated)
			generate := exec.Command("go", "generate", "./...")
			generate.Dir = fixture
			generate.Env = append(fixtureBuildEnv(), "GOWORK=off")
			if output, err := timedFixtureOutput(t, "router generation", generate); err != nil {
				t.Fatalf("generation: %v\n%s", err, output)
			}
			build := exec.Command(filepath.Join(web, "node_modules", ".bin", "vp"), "build")
			build.Dir = web
			build.Env = append(fixtureBuildEnv(), "GOWORK=off")
			output, err := timedFixtureOutput(t, "router production build", build)
			t.Logf("%s production build:\n%s", mode, output)
			if err != nil {
				t.Fatalf("production build: %v\n%s", err, output)
			}
			writeFixtureFile(t, filepath.Join(web, "dist.go"), "package web\nimport \"embed\"\n//go:embed all:build\nvar Build embed.FS\n")
			writeFixtureFile(t, filepath.Join(fixture, "internal", "server", "server.go"), hashRouterServer)
			writeFixtureFile(t, filepath.Join(fixture, "internal", "server", "server_test.go"), strings.Replace(hashRouterConsumer, "MODE245", mode, 1))
			writeFixtureFile(t, filepath.Join(fixture, "cmd", "server", "main.go"), `package main
import("log";"net/http";"github.com/tylergannon/skgo/example/internal/server")
func main(){handler,_,err:=server.Handler();if err!=nil{log.Fatal(err)};log.Println("http://127.0.0.1:4185");log.Fatal(http.ListenAndServe("127.0.0.1:4185",handler))}
`)
			consumer := exec.Command("go", "test", "-count=1", "-v", "./internal/server", "-run", "^TestEmbeddedRouterDocuments$")
			consumer.Dir = fixture
			consumer.Env = append(fixtureBuildEnv(), "GOWORK=off")
			output, err = timedFixtureOutput(t, "embedded router handler", consumer)
			t.Logf("%s embedded handler:\n%s", mode, output)
			if err != nil || bytes.Contains(output, []byte("--- SKIP:")) || !bytes.Contains(output, []byte("--- PASS: TestEmbeddedRouterDocuments")) {
				t.Fatalf("embedded router contract: %v\n%s", err, output)
			}
			if retained := os.Getenv("SKGO_HASH_FIXTURE_DIR"); retained != "" {
				t.Logf("retained production %s fixture: %s", mode, fixture)
			}
		})
	}
}

const hashRouterServer = `package server
import (
 "fmt"
 "io/fs"
 "net/http"
 "github.com/tylergannon/skgo"
 generated "github.com/tylergannon/skgo/example/internal/skgo"
 "github.com/tylergannon/skgo/example/web"
)
func Handler()(http.Handler,skgo.Manifest,error){
 dist,err:=fs.Sub(web.Build,"build");if err!=nil{return nil,skgo.Manifest{},err}
 manifest,err:=skgo.ReadManifest(dist);if err!=nil{return nil,manifest,err}
 loads,err:=skgo.NewLoads(manifest.LoadConfig("http://127.0.0.1:4185"),generated.Loads()...);if err!=nil{return nil,manifest,err}
 remotes,err:=skgo.NewRemotes(manifest.RemoteConfig("http://127.0.0.1:4185"),generated.Remotes()...);if err!=nil{return nil,manifest,err}
 renderer,err:=skgo.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{Runtimes:1});if err!=nil{return nil,manifest,err}
 pages,err:=skgo.NewStaticHandler(dist,skgo.WithSSR(renderer));if err!=nil{return nil,manifest,err}
 // An ordinary Go endpoint remains outside Kit's forbidden hash-mode server
 // route graph and continues to precede the document fallback.
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /literal-endpoint",func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","text/plain");w.WriteHeader(209);fmt.Fprint(w,"GO_ENDPOINT_245")})
 mux.Handle("/",pages)
 return mux,manifest,nil
}
`

const hashRouterConsumer = `package server
import("io";"net/http";"net/http/httptest";"strings";"testing")
func TestEmbeddedRouterDocuments(t *testing.T){
 const mode="MODE245"
 handler,manifest,err:=Handler();if err!=nil{t.Fatal(err)}
 if manifest.RouterType!=mode {t.Fatalf("adapter router type=%q want %q",manifest.RouterType,mode)}
 for _,pathname:=range []string{"/","/about","/deep/document"}{
  response:=httptest.NewRecorder();handler.ServeHTTP(response,httptest.NewRequest("GET","http://127.0.0.1:4185"+pathname,nil))
  body:=response.Body.String()
  if mode=="hash"{
   if response.Code!=200||!strings.HasPrefix(response.Header().Get("Content-Type"),"text/html")||!strings.Contains(body,"HASH_ROUTER_SHELL_42")||!strings.Contains(body,"/entry/start.")||!strings.Contains(body,"/entry/app.")||strings.Contains(body,"HASH_ROUTER_HOME_42")||strings.Contains(body,"HASH_ROUTER_ABOUT_42"){
    t.Errorf("hash document %s: status=%d headers=%v body=%s",pathname,response.Code,response.Header(),body)
   }
  }else{
   wantStatus,wantLiteral:=200,"HASH_ROUTER_HOME_42"
   if pathname=="/about"{wantLiteral="HASH_ROUTER_ABOUT_42"}
   if pathname=="/deep/document"{wantStatus=404;wantLiteral="Not Found"}
   if response.Code!=wantStatus||!strings.Contains(body,wantLiteral){t.Errorf("pathname document %s: status=%d body=%s",pathname,response.Code,body)}
  }
 }
 if mode=="hash"{for _,path:=range manifest.Prerendered{if path!="/"{t.Fatalf("hash build prerendered authored route %s",path)}}}
 for _,fixture:=range []struct{path string;status int;body string}{{"/asset-245.txt",200,"HASH_ASSET_245\n"},{"/literal-endpoint",209,"GO_ENDPOINT_245"},{"/_app/missing.js",404,""}}{
  response:=httptest.NewRecorder();handler.ServeHTTP(response,httptest.NewRequest(http.MethodGet,fixture.path,nil));body,_:=io.ReadAll(response.Result().Body)
  if response.Code!=fixture.status||string(body)!=fixture.body{t.Errorf("route precedence %s: status=%d body=%s",fixture.path,response.Code,body)}
 }
}
`
