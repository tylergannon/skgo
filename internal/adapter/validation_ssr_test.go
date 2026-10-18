package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProductionValidationBoundaryKeepsPublicBody(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/validation.remote.go"), `package routes
import("context";"os";"github.com/tylergannon/skgo")
func record() { if err:=os.WriteFile(os.Getenv("VALIDATION_BODY_RECEIPT"),[]byte("body ran"),0600);err!=nil{panic(err)} }
func rejected(ctx context.Context, input string)(string,error){record();return input,nil}
func rejectedLive(ctx context.Context,input string,yield func(string)error)error{record();return yield(input)}
func rejectedBatch(ctx context.Context,input []string)([]string,error){record();return input,nil}
var _=skgo.Query(rejected)
var _=skgo.LiveQuery(rejectedLive)
var _=skgo.BatchQuery(rejectedBatch)
`)
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/+page.svelte"), `<script lang="ts">import{rejected,rejectedLive,rejectedBatch}from'./validation.remote';</script>
<h1>Validation boundary fixture</h1>
<svelte:boundary>{await rejected(42 as any)}{#snippet failed(error)}<p id="query">{JSON.stringify(error)}</p>{/snippet}</svelte:boundary>
<svelte:boundary>{await rejectedLive(42 as any)}{#snippet failed(error)}<p id="live">{JSON.stringify(error)}</p>{/snippet}</svelte:boundary>
<svelte:boundary>{await rejectedBatch(42 as any)}{#snippet failed(error)}<p id="batch">{JSON.stringify(error)}</p>{/snippet}</svelte:boundary>`)
	generateMinimalInputsApp(t, fixture)
	if output, err := buildMinimalInputsApp(t, fixture); err != nil {
		t.Fatalf("production fixture build: %v\n%s", err, output)
	}
	writeFixtureFile(t, filepath.Join(fixture, "validation_test.go"), validationSSRContract)
	command := exec.Command("go", "test", ".", "-run", "^TestValidationBoundaries$", "-count=1", "-v")
	command.Dir, command.Env = fixture, inputsTestEnv(fixture, "")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("production handler: %v\n%s", err, output)
	} else {
		t.Logf("%s", output)
	}
}

const validationSSRContract = `package fixture_test
import("context";"encoding/json";"html";"net/http/httptest";"os";"path/filepath";"reflect";"regexp";"strings";"sync/atomic";"testing";"github.com/tylergannon/skgo";generated "github.com/tylergannon/skgo/example/internal/skgo")
func TestValidationBoundaries(t *testing.T){
 receipt:=filepath.Join(t.TempDir(),"remote-body");t.Setenv("VALIDATION_BODY_RECEIPT",receipt)
 dist:=os.DirFS("web/build");manifest,err:=skgo.ReadManifest(dist);if err!=nil{t.Fatal(err)}
 var hooks atomic.Int32
 cfg:=manifest.RemoteConfig("http://127.0.0.1:8080");cfg.HandleError=func(_ context.Context,caught skgo.CaughtError)map[string]any{
  hooks.Add(1);if caught.Kind!="validation"||caught.Status!=400||caught.Message!="Bad Request"||len(caught.Issues)!=1{t.Errorf("caught=%+v",caught)}
  return map[string]any{"status":422,"message":"PUBLIC_VALIDATION_671","code":"PUBLIC_CODE_782","supportId":"PUBLIC_SUPPORT_893"}
 }
 remotes,err:=skgo.NewRemotes(cfg,generated.Remotes()...);if err!=nil{t.Fatal(err)}
 loads,err:=skgo.NewLoads(manifest.LoadConfig("http://127.0.0.1:8080"));if err!=nil{t.Fatal(err)}
 renderer,err:=skgo.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{Runtimes:2});if err!=nil{t.Fatal(err)}
 pages,err:=skgo.NewStaticHandler(dist,skgo.WithSSR(renderer));if err!=nil{t.Fatal(err)}
 handler:=loads.Intercept(remotes.Intercept(pages));response:=httptest.NewRecorder();handler.ServeHTTP(response,httptest.NewRequest("GET","http://127.0.0.1:8080/",nil))
 document:=response.Body.String();if response.Code!=422||!strings.Contains(document,"Validation boundary fixture"){t.Fatalf("response %d: %s",response.Code,document)}
 want:=map[string]any{"status":float64(422),"message":"PUBLIC_VALIDATION_671","code":"PUBLIC_CODE_782","supportId":"PUBLIC_SUPPORT_893"}
 for _,kind:=range []string{"query","live","batch"}{
  matches:=regexp.MustCompile("<p id=\""+kind+"\">([^<]*)</p>").FindStringSubmatch(document)
  if len(matches)!=2{t.Errorf("missing %s boundary markup: %s",kind,document);continue}
  var got map[string]any;if err:=json.Unmarshal([]byte(html.UnescapeString(matches[1])),&got);err!=nil||!reflect.DeepEqual(got,want){t.Errorf("%s boundary: %s (%v); want %v",kind,matches[1],err,want)}
 }
 if hooks.Load()!=3{t.Errorf("hook calls=%d, want 3",hooks.Load())}
 if _,err:=os.Stat(receipt);!os.IsNotExist(err){t.Errorf("rejected application body ran: %v",err)}
}
`
