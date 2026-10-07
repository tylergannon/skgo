package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrerenderPredicateFailureRejectsPermissiveKitBuildAndDrainsOwner(t *testing.T) {
	for _, mode := range []string{"killed", "timeout", "worker-timeout"} {
		t.Run(mode, func(t *testing.T) {
			fixture := prepareMinimalInputsApp(t)
			config := filepath.Join(fixture, "internal", "skgo", "config.go")
			writeFixtureFile(t, config, "package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app --hook-package github.com/tylergannon/skgo/example/internal/serverhooks\n")
			writeFixtureFile(t, filepath.Join(fixture, "internal", "serverhooks", "handle.go"), `package serverhooks
import (
 "context"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "github.com/tylergannon/skgo"
 "github.com/tylergannon/skgo/example/internal/skgo/params"
)
var Handle = params.Middleware(func(ctx context.Context,event params.RequestEvent,resolve params.Resolve)(*http.Response,error){
 return resolve(ctx,event,skgo.ResolveOptions{Preload:func(input skgo.PreloadInput)bool{
  exe,_:=os.Executable()
  receipt:=fmt.Sprintf("%d\n%d\n%s\n",os.Getpid(),os.Getpid(),filepath.Dir(exe))
  if err:=os.WriteFile(os.Getenv("SKGO_LIFECYCLE_RECEIPT"),[]byte(receipt),0600);err!=nil{panic(err)}
  if os.Getenv("SKGO_PREDICATE_FAILURE_MODE")=="killed" {process,_:=os.FindProcess(os.Getpid());process.Kill();select{}}
  <-ctx.Done()
  os.WriteFile(os.Getenv("SKGO_TIMEOUT_CANCELLED"),[]byte("cancelled\n"),0600)
  return true
 }})
})
`)
			writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.ts"), "export const prerender=true;\n")
			configPath := filepath.Join(fixture, "web", "vite.config.ts")
			source, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			updated := strings.Replace(string(source), "adapter: skgo(),", "adapter: skgo(),\n prerender: {handleHttpError: 'ignore', handleUnseenRoutes: 'ignore'},", 1)
			if updated == string(source) {
				t.Fatal("fixture config did not select permissive error policy")
			}
			writeFixtureFile(t, configPath, updated)
			cancelled := filepath.Join(fixture, "predicate-cancelled")
			env := replaceEnv(inputsTestEnv(fixture, ""), "SKGO_TIMEOUT_CANCELLED", cancelled)
			env = replaceEnv(env, "SKGO_PREDICATE_FAILURE_MODE", mode)
			generate := exec.Command("go", "generate", "./...")
			generate.Dir, generate.Env = fixture, env
			if output, err := generate.CombinedOutput(); err != nil {
				t.Fatalf("generate: %v\n%s", err, output)
			}
			// Delay only the owner's service deadline in a disposable adapter to
			// isolate the worker notice from the independent HTTP timeout.
			if mode == "worker-timeout" {
				adapterRoot, err := filepath.Abs(".")
				if err != nil {
					t.Fatal(err)
				}
				copyRoot := filepath.Join(fixture, "adapter")
				copyFixtureTree(t, adapterRoot, copyRoot)
				modulePath := filepath.Join(copyRoot, "skgo-adapter", "prerender.js")
				module, err := os.ReadFile(modulePath)
				if err != nil {
					t.Fatal(err)
				}
				updated := strings.Replace(string(module), "const CALLBACK_TIMEOUT = 30_000;", "const CALLBACK_TIMEOUT = isMainThread ? 60_000 : 30_000;", 1)
				if updated == string(module) {
					t.Fatal("worker-timeout injection did not apply")
				}
				writeFixtureFile(t, modulePath, updated)
				link := filepath.Join(fixture, "web", "node_modules", "@skgo", "sveltekit-adapter")
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(copyRoot, link); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("node", "-e", inputsBuildProgram())
			cmd.Dir, cmd.Env = filepath.Join(fixture, "web"), env
			started := time.Now()
			tracked := startInputsTrackedCommand(t, cmd, fixture)
			err, timedOut := tracked.wait(60 * time.Second)
			output := tracked.output.String()
			t.Logf("%s native predicate build:\n%s", mode, output)
			if timedOut || err == nil || !strings.Contains(output, "BUILD_APP_REJECTED:") || strings.Contains(output, "BUILD_APP_RESOLVED") {
				t.Fatalf("predicate failure became build success: %v deadline=%v\n%s", err, timedOut, output)
			}
			if mode != "killed" {
				if !strings.Contains(output, "timed out after 30000ms") {
					t.Fatalf("worker timeout not observed: %s", output)
				}
				if data, err := os.ReadFile(cancelled); err != nil || string(data) != "cancelled\n" {
					t.Fatalf("callback context did not cancel: %s %v", data, err)
				}
				if mode == "worker-timeout" && !strings.Contains(output, "BUILD_APP_REJECTED:Error: skgo prerender preload predicate timed out after 30000ms") {
					t.Fatalf("worker failure notice did not terminally fail the owner: %s", output)
				}
			} else if time.Since(started) > 15*time.Second || !(strings.Contains(output, "service exited unexpectedly") || strings.Contains(output, "preload resolve callback failed: fetch failed")) {
				t.Fatalf("dead service was not reported promptly: %s", output)
			}
			if _, err := os.Stat(filepath.Join(fixture, "web", "build")); !os.IsNotExist(err) {
				t.Fatalf("failed build left successful adapter output: %v", err)
			}
			assertInputsDrainComplete(t, fixture, output)
		})
	}
}

func TestPrerenderNilCallbacksProduceNoPredicateTraffic(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	writeFixtureFile(t, filepath.Join(fixture, "internal", "skgo", "config.go"), "package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app --hook-package github.com/tylergannon/skgo/example/internal/serverhooks\n")
	writeFixtureFile(t, filepath.Join(fixture, "internal", "serverhooks", "handle.go"), "package serverhooks\nimport \"github.com/tylergannon/skgo/example/internal/skgo/params\"\nvar Handle params.Middleware\n")
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.ts"), "export const prerender=true;\n")
	env := inputsTestEnv(fixture, "")
	generate := exec.Command("go", "generate", "./...")
	generate.Dir, generate.Env = fixture, env
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v\n%s", err, output)
	}
	// Observe the real owner's channel while building; don't change or replace
	// its callback handler. The ordinary generated Go main stays untouched.
	program := `import {createBuilder} from 'vite';
import {createPrerenderOwner} from '@skgo/sveltekit-adapter/skgo-adapter/prerender.js';
import assert from 'node:assert/strict';
const builder=await createBuilder({root:process.cwd(),configFile:'vite.config.ts',logLevel:'silent',builder:{}});
const owner=createPrerenderOwner(process.cwd());
assert.ok(owner);
let published=0,predicates=0;
const publish=owner.publish;
owner.publish=()=>{publish();published++;const receive=owner.channel.onmessage;owner.channel.onmessage=(event)=>{if(event.data?.type==='predicate')predicates++;return receive(event);};};
try {await builder.buildApp();assert.equal(published,1);assert.equal(predicates,0);assert.equal(owner.channel,null);assert.equal(process.env.SKGO_PRERENDER_PREDICATE_CHANNEL,undefined);console.log('NIL_CALLBACK_TRAFFIC:0 OWNER_CHANNEL_CLOSED');}
finally{await builder.close?.();}
`
	// The adapter exports only its public entrypoints; import its on-disk
	// internal module by absolute URL, as the existing owner tests do.
	root, err := filepath.Abs("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	program = strings.Replace(program, "'@skgo/sveltekit-adapter/skgo-adapter/prerender.js'", "'file://"+filepath.ToSlash(root)+"'", 1)
	cmd := exec.Command("node", "-e", program)
	cmd.Dir, cmd.Env = filepath.Join(fixture, "web"), env
	tracked := startInputsTrackedCommand(t, cmd, fixture)
	err, timedOut := tracked.wait(30 * time.Second)
	output := tracked.output.String()
	if err != nil || timedOut || !strings.Contains(output, "NIL_CALLBACK_TRAFFIC:0 OWNER_CHANNEL_CLOSED") {
		t.Fatalf("nil callback build: %v deadline=%v\n%s", err, timedOut, output)
	}
	body, err := os.ReadFile(filepath.Join(fixture, "web", "build", "prerendered", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "<h1>Minimal declared Inputs fixture</h1>") || !strings.Contains(string(body), `rel="modulepreload"`) {
		t.Fatalf("Kit defaults lost: %s", body)
	}
	t.Logf("nil callbacks real build: %s", output)
}
