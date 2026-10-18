package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestServerInitBeforeServingDevelopmentAndProduction(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	// Startup still precedes all Go requests when page rendering is disabled.
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/+page.ts"), "export const ssr = false;\n")
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/+page.svelte"), `<script>import{startupReceipt}from'./receipt.remote';</script><h1>Startup fixture</h1><button onclick={()=>startupReceipt()}>Read startup receipt</button>`)
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/receipt.remote.go"), `package routes
import("context";"os";"github.com/tylergannon/skgo")
func startupReceipt(context.Context)(string,error){body,err:=os.ReadFile(os.Getenv("SERVER_INIT_RECEIPT"));return string(body),err}
var _=skgo.Query(startupReceipt)
`)
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/receipt/server.go"), `package receipt
import("net/http";"os";"github.com/tylergannon/skgo")
func getReceipt(w http.ResponseWriter,r *http.Request){body,err:=os.ReadFile(os.Getenv("SERVER_INIT_RECEIPT"));if err!=nil{http.Error(w,err.Error(),500);return};w.Write(body)}
var _=skgo.GET(getReceipt)
`)
	generateMinimalInputsApp(t, fixture)
	if output, err := buildMinimalInputsApp(t, fixture); err != nil {
		t.Fatalf("startup fixture build: %v\n%s", err, output)
	}
	writeFixtureFile(t, filepath.Join(fixture, "startup_test.go"), serverInitContract)
	command := exec.Command("go", "test", ".", "-run", "^TestStartup", "-count=1", "-v")
	command.Dir, command.Env = fixture, inputsTestEnv(fixture, "")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("startup handlers: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
}

const serverInitContract = `package fixture_test
import("errors";"fmt";"io";"net/http";"net/http/httptest";"os";"path/filepath";"strings";"sync/atomic";"testing";"time";"github.com/tylergannon/skgo";generated "github.com/tylergannon/skgo/example/internal/skgo")
func newApplication(mode string,init skgo.ServerInit,receipt string)(http.Handler,*skgo.NativeDevelopment,error){
 dist:=os.DirFS("web/build");m,err:=skgo.ReadManifest(dist);if err!=nil{return nil,nil,err}
 var development *skgo.NativeDevelopment;if mode=="development"{development=skgo.NewNativeDevelopment("startup-secret")}
 remotes,err:=skgo.NewRemotes(m.RemoteConfig("http://127.0.0.1:8080"),generated.Remotes()...);if err!=nil{return nil,development,err}
 loads,err:=skgo.NewLoads(m.LoadConfig("http://127.0.0.1:8080"));if err!=nil{return nil,development,err}
 endpoints,err:=skgo.NewEndpoints(m.EndpointConfig("http://127.0.0.1:8080"),generated.Endpoints()...);if err!=nil{return nil,development,err}
 renderer,err:=development.NewSSR(dist,m,loads,remotes,skgo.SSROptions{Init:init,Runtimes:3});if err!=nil{return nil,development,err}
 pages,err:=development.NewStaticHandler(dist,skgo.WithSSR(renderer));if err!=nil{return nil,development,err}
 handler:=loads.Intercept(remotes.Intercept(endpoints.Intercept(pages)))
 if development!=nil{h,_,e:=development.Handler(handler);return h,development,e};return handler,nil,nil
}
func TestStartupWaitsOncePerInstance(t *testing.T){
 for _,mode:=range []string{"production","development"}{t.Run(mode,func(t *testing.T){
  receipt:=filepath.Join(t.TempDir(),"startup");t.Setenv("SERVER_INIT_RECEIPT",receipt)
  var calls atomic.Int32;entered,release:=make(chan struct{}),make(chan struct{})
  init:=func()error{n:=calls.Add(1);if n==1{close(entered);<-release};return os.WriteFile(receipt,[]byte(fmt.Sprintf("STARTUP_731_INSTANCE_%d",n)),0600)}
  type answer struct{handler http.Handler;err error};constructed:=make(chan answer,1)
  go func(){handler,_,err:=newApplication(mode,init,receipt);constructed<-answer{handler,err}}()
  select{case<-entered:case a:=<-constructed:t.Fatalf("construction returned before init entered: %v",a.err);case<-time.After(5*time.Second):t.Fatal("init never ran")}
  select{case a:=<-constructed:t.Fatalf("application became available before init completed: %v",a.err);default:}
  close(release)
  var a answer;select{case a=<-constructed:case<-time.After(5*time.Second):t.Fatal("constructor did not finish")};if a.err!=nil{t.Fatal(a.err)}
  check:=func(handler http.Handler,literal string){
   server:=httptest.NewServer(handler);defer server.Close()
   for repeat:=0;repeat<2;repeat++{for _,path:=range []string{"/receipt","/_app/remote/"+generated.Remotes()[0].ID()}{
    response,err:=server.Client().Get(server.URL+path);if err!=nil{t.Fatal(err)};body,err:=io.ReadAll(response.Body);response.Body.Close()
    if err!=nil||response.StatusCode!=200||!strings.Contains(string(body),literal){t.Fatalf("%s: %d %s (%v)",path,response.StatusCode,body,err)}
   }}
  }
  check(a.handler,"STARTUP_731_INSTANCE_1");if calls.Load()!=1{t.Fatalf("first instance calls=%d",calls.Load())}
  second,_,err:=newApplication(mode,init,receipt);if err!=nil{t.Fatal(err)};check(second,"STARTUP_731_INSTANCE_2")
  if calls.Load()!=2{t.Fatalf("two independent instances calls=%d",calls.Load())}
 })}
}
func TestStartupFailurePreventsServing(t *testing.T){
 for _,mode:=range []string{"production","development"}{t.Run(mode,func(t *testing.T){
  cause:=errors.New("DATABASE_UNAVAILABLE_842");var calls atomic.Int32
  handler,development,err:=newApplication(mode,func()error{calls.Add(1);return cause},"")
  if handler!=nil||!errors.Is(err,cause)||err.Error()!="skgo: server init failed: DATABASE_UNAVAILABLE_842"||calls.Load()!=1{t.Fatalf("handler=%v err=%v calls=%d",handler,err,calls.Load())}
  if development!=nil{handler,_,err:=development.Handler(http.NotFoundHandler());if handler!=nil||err==nil{t.Fatal("failed native instance published request callbacks")}}
 })}
}
`
