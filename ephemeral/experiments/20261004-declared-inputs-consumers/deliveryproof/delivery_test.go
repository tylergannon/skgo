package deliveryproof

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const proofRoot = "/Users/tyler/Codex/2026-10-03/task-8/prerender-inputs-proof"

func write(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func deliveryRun(t *testing.T, cwd, logfile string, extra []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GO111MODULE=on", "GOWORK=off", "GOCACHE=/tmp/skgo-inputs-proof-go-cache", "CI=1", "XDG_DATA_HOME="+proofRoot+"/delivery/data", "XDG_CACHE_HOME="+proofRoot+"/delivery/cache", "PNPM_HOME="+proofRoot+"/delivery/pnpm-home", "npm_config_store_dir="+proofRoot+"/delivery/store-bootstrap", "PATH="+proofRoot+"/delivery/tools/node_modules/.bin:"+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, extra...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	file, err := os.Create(filepath.Join(cwd, logfile))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(file, &output)
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Logf("owned command pid/group=%d cwd=%s args=%q", cmd.Process.Pid, cwd, args)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
		t.Errorf("owned command exceeded four-minute deadline")
	}
	return output.String(), err
}

func TestDeliveryBootstrapNativeTools(t *testing.T) {
	root := filepath.Join(proofRoot, "delivery/tools")
	write(t, root, "package.json", `{"name":"skgo-inputs-proof-tools","private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"vite-plus":"1.0.0","sv":"1.0.1"}}`)
	if out, err := deliveryRun(t, root, "install.log", nil, "pnpm", "install", "--no-frozen-lockfile"); err != nil {
		t.Fatalf("native tools install: %v\n%s", err, out)
	}
	out, err := deliveryRun(t, root, "versions.log", nil, "node_modules/.bin/vp", "--version")
	if err != nil || !strings.Contains(out, "vite-plus  v1.0.0") && !strings.Contains(out, "vite-plus  1.0.0") {
		t.Fatalf("owned VP1.0.0 unavailable: %v\n%s", err, out)
	}
}

type packedSDK struct{ sdk, adapter, addon, addonDir string }

func packSDK(t *testing.T, sdk, label string) packedSDK {
	t.Helper()
	root := filepath.Join(proofRoot, "delivery/packages", label)
	for _, part := range []string{"adapter", "sv"} {
		dest := filepath.Join(root, part)
		if err := os.MkdirAll(dest, 0755); err != nil {
			t.Fatal(err)
		}
		out, err := deliveryRun(t, filepath.Join(sdk, "internal", part), "independent-pack.log", nil, "pnpm", "pack", "--pack-destination", dest)
		if err != nil {
			t.Fatalf("native pack %s: %v\n%s", part, err, out)
		}
	}
	one := func(part string) string {
		files, err := filepath.Glob(filepath.Join(root, part, "*.tgz"))
		if err != nil || len(files) != 1 {
			t.Fatalf("pack %s files=%v err=%v", part, files, err)
		}
		return files[0]
	}
	p := packedSDK{sdk: sdk, adapter: one("adapter"), addon: one("sv"), addonDir: filepath.Join(root, "sv/extracted/package")}
	f, err := os.Open(p.addon)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	r := tar.NewReader(z)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		clean := filepath.Clean(h.Name)
		if !strings.HasPrefix(clean, "package/") || strings.Contains(clean, "..") {
			t.Fatalf("unexpected packed path%s", clean)
		}
		target := filepath.Join(root, "sv/extracted", clean)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if h.Typeflag != tar.TypeReg {
			t.Fatalf("unexpected packed type%d", h.Typeflag)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(target, body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

const scaffoldSeam = `package newapp
import("os";"testing")
func TestIndependentPackedCreateFinish(t *testing.T){
 args:=[]string{"--template","minimal","--types",os.Getenv("SKGO_DELIVERY_TYPES"),"--add","vitest=usages:unit"};if os.Getenv("SKGO_DELIVERY_STORYBOOK")=="selected"{args=append(args,"storybook")}
 result,err:=Create(Options{Dir:os.Getenv("SKGO_DELIVERY_APP"),Module:"example.com/prerenderconsumer",App:"prerenderconsumer",Origin:"https://prerender-proof.invalid",SkgoVersion:"v0.16.4",SkgoReplace:os.Getenv("SKGO_DELIVERY_SDK"),SVAddonSpec:"file:"+os.Getenv("SKGO_DELIVERY_ADDON_DIR"),AdapterSpec:"file:"+os.Getenv("SKGO_DELIVERY_ADAPTER"),SvArgs:args,Interactive:false,VP:os.Getenv("SKGO_DELIVERY_VP"),Stdout:os.Stdout,Stderr:os.Stderr,addonStage:os.Getenv("SKGO_DELIVERY_STAGE")});if err!=nil{t.Fatal(err)};t.Logf("real Create/finish result=%+v",result)
}
`

func scaffold(t *testing.T, p packedSDK, name, types, storybook string) string {
	t.Helper()
	app := filepath.Join(proofRoot, "delivery-fixtures", name)
	if _, err := os.Stat(app); err == nil {
		t.Fatalf("refusing to overwrite retained scaffold%s", app)
	}
	write(t, p.sdk, "internal/newapp/independent_delivery_test.go", scaffoldSeam)
	env := []string{"SKGO_DELIVERY_APP=" + app, "SKGO_DELIVERY_TYPES=" + types, "SKGO_DELIVERY_STORYBOOK=" + storybook, "SKGO_DELIVERY_SDK=" + p.sdk, "SKGO_DELIVERY_ADAPTER=" + p.adapter, "SKGO_DELIVERY_ADDON_DIR=" + p.addonDir, "SKGO_DELIVERY_VP=" + filepath.Join(proofRoot, "delivery/tools/node_modules/.bin/vp"), "SKGO_DELIVERY_STAGE=" + app + "-addon-stage"}
	out, err := deliveryRun(t, p.sdk, "independent-scaffold-"+name+".log", env, "go", "test", "-count=1", "-run", "^TestIndependentPackedCreateFinish$", "-v", "./internal/newapp")
	if err != nil {
		t.Fatalf("actual native scaffold %s: %v\n%s", name, err, out)
	}
	return app
}

func TestDeliveryRetainStockScaffold(t *testing.T) {
	p := packSDK(t, filepath.Join(proofRoot, "delivery/baseline-sdk"), "baseline-v0.16.4")
	app := scaffold(t, p, "stock-v0.16.4", "ts", "default")
	if _, err := os.Stat(filepath.Join(app, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js")); err != nil {
		t.Fatal(err)
	}
}

func candidate(t *testing.T) string {
	t.Helper()
	p := os.Getenv("SKGO_DELIVERY_SDK")
	if p == "" {
		t.Fatal("immutable SKGO_DELIVERY_SDK is required")
	}
	return p
}

func TestDeliveryPackImmutableCandidate(t *testing.T) {
	p := packSDK(t, candidate(t), filepath.Base(candidate(t)))
	t.Logf("actual native packed candidate SDK=%s adapter=%s addon=%s", p.sdk, p.adapter, p.addon)
}
func content(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func sha(body string) string { h := sha256.Sum256([]byte(body)); return hex.EncodeToString(h[:]) }
func must(t *testing.T, cwd, log string, env []string, args ...string) string {
	t.Helper()
	out, err := deliveryRun(t, cwd, log, env, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

const inputsGo = `package lib
import("context";"fmt";"os";"time";"github.com/tylergannon/skgo";"github.com/tylergannon/polytype/devalue")
func note(value string){if path:=os.Getenv("SKGO_INPUTS_RECEIPT");path!=""{f,e:=os.OpenFile(path,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0644);if e!=nil{panic(e)};defer f.Close();fmt.Fprintln(f,value)}}
func catalogInputs()([]string,error){note("inputs:catalog");time.Sleep(500*time.Millisecond);return []string{"atlas","beacon"},nil}
func catalog(_ context.Context,value string)(string,error){note("body:catalog:"+value);return "build:"+value,nil}
var _=skgo.Prerender(catalog,skgo.PrerenderOptions{Inputs:catalogInputs})
func emptyInputs()([]devalue.UndefinedValue,error){note("inputs:empty");return []devalue.UndefinedValue{},nil}
func empty(_ context.Context)(string,error){note("body:empty");return "build:empty",nil}
var _=skgo.Prerender(empty,skgo.PrerenderOptions{Inputs:emptyInputs})
`

const guardBlockedLoad = `package routes
import("context";"fmt";"os";"os/signal";"path/filepath";"syscall";"time";"github.com/tylergannon/skgo")
type PageData struct {Value string}
func pageLoad(_ context.Context)(PageData,error){signal.Ignore(syscall.SIGTERM);exe,_:=os.Executable();os.WriteFile(os.Getenv("SKGO_BLOCK_READY"),[]byte(fmt.Sprintf("%d %s\n",os.Getpid(),filepath.Dir(exe))),0644);for{f,_:=os.OpenFile(os.Getenv("SKGO_BLOCK_LATE"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0644);fmt.Fprintln(f,"live owned load");f.Close();time.Sleep(25*time.Millisecond)}}
var _=skgo.Load(pageLoad)
`

const guardJoinDriver = `(async()=>{const {createBuilder}=await import('vite');const {existsSync,readFileSync,writeFileSync}=await import('node:fs');const builder=await createBuilder({configFile:'vite.config.ts'});let rejected;try{await builder.buildApp()}catch(error){rejected=error}if(!rejected||!String(rejected).includes('SKGO_KIT_PRERENDER_QUEUE'))throw new Error('wrong guarded native rejection:'+rejected);const [text,dir]=readFileSync(process.env.SKGO_BLOCK_READY,'utf8').trim().split(/\s+/);const pid=Number(text);let alive=false;try{process.kill(-pid,0);alive=true}catch(error){if(error.code!=='ESRCH')throw error}const receipt={original:String(rejected),pid,dir,childrenAtRejection:alive?1:0,tempDirAtRejection:existsSync(dir)};const before=readFileSync(process.env.SKGO_BLOCK_LATE,'utf8');await new Promise(r=>setTimeout(r,250));receipt.lateIO=before===readFileSync(process.env.SKGO_BLOCK_LATE,'utf8')?0:1;writeFileSync(process.env.SKGO_BLOCK_RESULT,JSON.stringify(receipt));if(receipt.childrenAtRejection||receipt.tempDirAtRejection||receipt.lateIO)throw new Error('guard rejected before owner cleanup:'+JSON.stringify(receipt));console.log('GUARD_NATIVE_WORKER_JOINED '+JSON.stringify(receipt));process.exitCode=19})().catch(error=>{console.error(error);process.exitCode=1});`

func TestDeliveryNativeWorkerGuardJoinsBlockedOwnedGoLoad(t *testing.T) {
	sdk := os.Getenv("SKGO_DELIVERY_SDK")
	if sdk == "" {
		sdk = filepath.Join(proofRoot, "delivery/queue-sdk-0ab6303")
	}
	label := filepath.Base(sdk)
	if suffix := os.Getenv("SKGO_DELIVERY_GUARD_SUFFIX"); suffix != "" {
		label += "-" + suffix
	}
	p := packSDK(t, sdk, "guard-"+label)
	app := filepath.Join(proofRoot, "delivery-fixtures/guard-native-worker-"+label)
	authoredCopy(t, filepath.Join(proofRoot, "delivery-fixtures/stock-v0.16.4"), app)
	mod := strings.ReplaceAll(content(t, filepath.Join(app, "go.mod")), filepath.Join(proofRoot, "delivery/baseline-sdk"), sdk)
	write(t, app, "go.mod", mod)
	must(t, app, "upgrade-sdk-tidy.log", nil, "go", "mod", "tidy")
	updateManifest(t, app, func(m map[string]any) {
		m["devDependencies"].(map[string]any)["@skgo/sveltekit-adapter"] = "file:" + p.adapter
	})
	write(t, app, "web/src/routes/page.server.go", guardBlockedLoad)
	write(t, app, "web/src/lib/fixture.remote.go", `package lib
import("context";"fmt";"os";"github.com/tylergannon/skgo")
func note(value string){f,_:=os.OpenFile(os.Getenv("SKGO_INPUTS_RECEIPT"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0644);fmt.Fprintln(f,value);f.Close()}
func catalogInputs()([]string,error){note("inputs:catalog");return []string{"atlas"},nil}
func catalog(_ context.Context,_ string)(string,error){note("body:catalog");return "unexpected",nil}
var _=skgo.Prerender(catalog,skgo.PrerenderOptions{Inputs:catalogInputs})`)
	write(t, app, "web/src/routes/+layout.ts", `import '../lib/fixture.remote';`)
	write(t, app, "web/src/routes/+page.ts", `export const prerender=true;`)
	write(t, app, "web/src/routes/+page.svelte", `<h1>Owned blocked Go load</h1>`)
	config := content(t, filepath.Join(app, "web/vite.config.ts"))
	config = strings.Replace(config, "experimental: { remoteFunctions: true },", "experimental: { remoteFunctions: true },prerender:{concurrency:1,handleHttpError:'ignore'},", 1)
	write(t, app, "web/vite.config.ts", config)
	must(t, filepath.Join(app, "web"), "native-unpatched-install.log", nil, "pnpm", "install", "--no-frozen-lockfile")
	env := []string{"SKGO_BLOCK_READY=" + filepath.Join(app, "body.ready"), "SKGO_BLOCK_LATE=" + filepath.Join(app, "late.log"), "SKGO_BLOCK_RESULT=" + filepath.Join(app, "rejection.json"), "SKGO_INPUTS_RECEIPT=" + filepath.Join(app, "inputs-ipc-body.log")}
	must(t, app, "guard-generate.log", env, "go", "generate", "./...")
	stub := content(t, filepath.Join(app, "web/src/lib/fixture.remote.ts"))
	needle := `$skgoRemoteInputs<string>("src/lib/fixture.remote.ts", "catalog")`
	if !strings.Contains(stub, needle) {
		t.Fatalf("real generated Inputs factory call missing:%s", stub)
	}
	wrapped := `(async()=>{const fs=await import('node:fs');const deadline=Date.now()+10000;while(!fs.existsSync(process.env.SKGO_BLOCK_READY!)){if(Date.now()>deadline)throw new Error('owned body readiness missing');await new Promise(r=>setTimeout(r,10))}const [pid]=fs.readFileSync(process.env.SKGO_BLOCK_READY!,'utf8').trim().split(/\s+/);process.kill(-Number(pid),0);return ` + needle + `;})()`
	write(t, app, "web/src/lib/fixture.remote.ts", strings.Replace(stub, needle, wrapped, 1))
	out, err := deliveryRun(t, filepath.Join(app, "web"), "guard-native-programmatic.log", env, "node", "-e", guardJoinDriver)
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 19 || !strings.Contains(out, "GUARD_NATIVE_WORKER_JOINED") {
		if b, readErr := os.ReadFile(filepath.Join(app, "body.ready")); readErr == nil {
			tokens := strings.Fields(string(b))
			if len(tokens) > 1 {
				if pid, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
					_ = syscall.Kill(-pid, syscall.SIGKILL)
				}
			}
		}
		t.Fatalf("guarded native build nonzero/drain:%v:%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(app, "inputs-ipc-body.log")); !os.IsNotExist(err) {
		t.Fatalf("Go Inputs IPC/body executed despite incompatible Kit:%v", err)
	}
	t.Logf("actual guard worker immediate rejection:%s", content(t, filepath.Join(app, "rejection.json")))
}

func authorInputs(t *testing.T, app, language string) {
	t.Helper()
	ext := "ts"
	script := `<script lang="ts">`
	if language == "jsdoc" {
		ext = "js"
		script = `<script>`
	}
	write(t, app, "web/src/lib/fixture.remote.go", inputsGo)
	write(t, app, "web/src/routes/+layout."+ext, `import '../lib/fixture.remote';`)
	write(t, app, "web/src/routes/+page."+ext, `export const prerender=true;`)
	write(t, app, "web/src/routes/+page.svelte", `<h1>Caller-free Inputs fixture</h1>`)
	write(t, app, "web/src/routes/consumer/+page."+ext, `export const prerender=false;`)
	write(t, app, "web/src/routes/consumer/+page.svelte", script+`import {catalog,empty} from '../../lib/fixture.remote';const a=await catalog('atlas');const e=await empty();</script><p data-testid="catalog">{a}</p><p data-testid="empty">{e}</p>`)
	write(t, app, "web/src/routes/prerender-helper/+page."+ext, `import {remoteInputs} from '@skgo/sveltekit-adapter/prerender';export const prerender=false;export async function load(){try{await remoteInputs('goja-control','remoteInputs');return {message:'inert helper unexpectedly returned'}}catch(error){return {message:error instanceof Error?error.message:String(error)}}}`)
	write(t, app, "web/src/routes/prerender-helper/+page.svelte", `<script>let {data}=$props();</script><p data-testid="helper">{data.message}</p>`)
}

func verifyInstalled(t *testing.T, app string) {
	t.Helper()
	for _, name := range []string{"@skgo/sveltekit-adapter", "@sveltejs/kit", "vite-plus"} {
		physical, err := filepath.EvalSymlinks(filepath.Join(app, "web/node_modules", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(physical, proofRoot+string(filepath.Separator)) {
			t.Fatalf("dependency%s escaped owned proof root:%s", name, physical)
		}
		if strings.Contains(physical, "/skgo-prerender-inputs/") || strings.Contains(physical, "/delivery/baseline-sdk/internal/") {
			t.Fatalf("dependency inherits checkout/source:%s", physical)
		}
		t.Logf("native installed %s resolves %s", name, physical)
	}
	const corrected = "400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5"
	if got := sha(content(t, filepath.Join(app, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js"))); got != corrected {
		t.Fatalf("installed queue%s want canonical%s", got, corrected)
	}
	must(t, app, "public-check.log", nil, "go", "tool", "skgo", "kit-patch", "--web", "web", "--check")
}

func buildAndAssert(t *testing.T, app, language, label string) {
	t.Helper()
	trace := filepath.Join(app, "calls-"+label+".log")
	env := []string{"SKGO_INPUTS_RECEIPT=" + trace}
	must(t, app, "generate-"+label+".log", env, "go", "generate", "./...")
	if _, err := os.Stat(trace); !os.IsNotExist(err) {
		t.Fatalf("producer/body executed during generate:%v", err)
	}
	hash := "3215r6"
	ext := "ts"
	if language == "jsdoc" {
		hash = "1vhm2m4"
		ext = "js"
	}
	if _, err := os.Stat(filepath.Join(app, "web/src/lib/fixture.remote."+ext)); err != nil {
		t.Fatal(err)
	}
	must(t, app, "build-"+label+".log", env, "just", "build")
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(content(t, trace)), "\n") {
		counts[line]++
	}
	for _, line := range []string{"inputs:catalog", "inputs:empty", "body:catalog:atlas", "body:catalog:beacon", "body:empty"} {
		if counts[line] != 1 {
			t.Fatalf("receipt%s count%d want literal1: %v", line, counts[line], counts)
		}
	}
	if len(counts) != 5 {
		t.Fatalf("unexpected producer/body receipts%v", counts)
	}
	for _, tc := range []struct{ suffix, key, value string }{{"catalog/WyJhdGxhcyJd", "catalog/WyJhdGxhcyJd", "build:atlas"}, {"catalog/WyJiZWFjb24iXQ", "catalog/WyJiZWFjb24iXQ", "build:beacon"}, {"empty", "empty/", "build:empty"}} {
		got := content(t, filepath.Join(app, "web/build/prerendered/_app/remote", hash, tc.suffix))
		want := fmt.Sprintf(`{"type":"result","data":"[{\"_\":1,\"p\":2},\"%s\",{\"%s/%s\":3},{\"v\":1}]"}`, tc.value, hash, tc.key)
		if got != want {
			t.Fatalf("literal native artifact %s got%s want%s", tc.suffix, got, want)
		}
	}
	probe := strings.ReplaceAll(compiledConsumer, "HASH", hash)
	write(t, app, "independent_consumer_test.go", probe)
	runtimeTrace := filepath.Join(app, "runtime-"+label+".log")
	must(t, app, "compiled-"+label+".log", []string{"SKGO_INPUTS_RECEIPT=" + runtimeTrace}, "go", "test", "-count=1", "-run", "^TestIndependentPackedCompiledHTTPAndSSR$", "-v", ".")
	if _, err := os.Stat(runtimeTrace); !os.IsNotExist(err) {
		t.Fatalf("producer/body executed in runtime:%v", err)
	}
	bundle := content(t, filepath.Join(app, "web/build/ssr/bundle.js"))
	for _, builtin := range []string{"node:child_process", "node:fs", "node:module", "node:crypto", "node:worker_threads"} {
		if strings.Contains(bundle, `node_module("`+builtin+`")`) {
			t.Fatalf("actual Goja bundle externalizes Node builtin%s", builtin)
		}
	}
}

const compiledConsumer = `package app_test
import("net/http/httptest";"os";"strings";"testing";app "example.com/prerenderconsumer")
func TestIndependentPackedCompiledHTTPAndSSR(t *testing.T){h,mode,e:=app.NewHandler(os.DirFS("web/build"),"","https://prerender-proof.invalid");if e!=nil{t.Fatal(e)};if mode!="production"{t.Fatal(mode)};for _,tc:=range []struct{path,want string}{{"/_app/remote/HASH/catalog/WyJhdGxhcyJd","build:atlas"},{"/_app/remote/HASH/catalog/WyJiZWFjb24iXQ","build:beacon"},{"/_app/remote/HASH/empty","build:empty"}}{w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET",tc.path,nil));if w.Code!=200||!strings.Contains(w.Body.String(),tc.want){t.Fatalf("HTTP %s:%d %s",tc.path,w.Code,w.Body)}};w:=httptest.NewRecorder();h.ServeHTTP(w,httptest.NewRequest("GET","/consumer",nil));if w.Code!=200{t.Fatalf("SSR:%d %s",w.Code,w.Body)};for _,want:=range []string{"<p data-testid=\"catalog\">build:atlas</p>","<p data-testid=\"empty\">build:empty</p>","HASH/catalog/WyJhdGxhcyJd","HASH/empty/"}{if !strings.Contains(w.Body.String(),want){t.Fatalf("SSR missing literal%s:%s",want,w.Body)}};helper:=httptest.NewRecorder();h.ServeHTTP(helper,httptest.NewRequest("GET","/prerender-helper",nil));if helper.Code!=200||!strings.Contains(helper.Body.String(),"<p data-testid=\"helper\">skgo: declared prerender inputs are build-only</p>"){t.Fatalf("real Goja inert-helper invocation:%d:%s",helper.Code,helper.Body)}}
`

func frozenReinstall(t *testing.T, app, label string) string {
	t.Helper()
	deps := filepath.Join(app, "web/node_modules")
	if err := os.RemoveAll(deps); err != nil {
		t.Fatal(err)
	}
	relocated := app + "-relocated-frozen"
	authoredCopy(t, app, relocated)
	store := filepath.Join(proofRoot, "delivery", "independent-store-"+label)
	if _, err := os.Stat(store); err == nil {
		t.Fatalf("frozen reinstall store not fresh:%s", store)
	}
	must(t, filepath.Join(relocated, "web"), "frozen-reinstall.log", []string{"PNPM_HOME=" + store}, "pnpm", "install", "--frozen-lockfile", "--store-dir", store)
	verifyInstalled(t, relocated)
	return relocated
}

func TestDeliveryFreshPackedTypeScriptAndJavaScript(t *testing.T) {
	p := packSDK(t, candidate(t), filepath.Base(candidate(t)))
	for _, tc := range []struct{ name, language, storybook string }{{"fresh-ts", "ts", "default"}, {"fresh-js", "jsdoc", "selected"}} {
		t.Run(tc.name, func(t *testing.T) {
			app := scaffold(t, p, tc.name+"-"+filepath.Base(p.sdk), tc.language, tc.storybook)
			verifyInstalled(t, app)
			authorInputs(t, app, tc.language)
			buildAndAssert(t, app, tc.language, "initial")
			frozen := frozenReinstall(t, app, tc.name+"-"+filepath.Base(p.sdk))
			buildAndAssert(t, frozen, tc.language, "frozen")
		})
	}
}

func authoredCopy(t *testing.T, source, dest string) {
	t.Helper()
	if _, err := os.Stat(dest); err == nil {
		t.Fatalf("refusing to overwrite retained fixture%s", dest)
	}
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".svelte-kit") {
			return filepath.SkipDir
		}
		for _, skip := range []string{"web/node_modules", "web/.svelte-kit", "web/build", "web/dist", "bin"} {
			if rel == skip && d.IsDir() {
				return filepath.SkipDir
			}
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected authored fixture file%s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
	write(t, dest, "web/build/.gitkeep", "")
}

func updateManifest(t *testing.T, app string, mutate func(map[string]any)) {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal([]byte(content(t, filepath.Join(app, "web/package.json"))), &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(manifest)
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, app, "web/package.json", string(body)+"\n")
}

func setPatchMap(t *testing.T, app, label string, m map[string]string) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(app, "web"), "config-"+label+".log", nil, "pnpm", "config", "set", "patchedDependencies", string(b), "--location=project", "--json")
}

func unrelatedSentinel(t *testing.T, app string) {
	t.Helper()
	source := filepath.Join(app, "independent-sentinel-package")
	write(t, source, "package.json", `{"name":"inputs-delivery-sentinel","version":"1.0.0","type":"module","main":"index.js"}`)
	write(t, source, "index.js", "export const sentinel = 'original';\n")
	dest := filepath.Join(app, "independent-packed-sentinel")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	must(t, source, "pack.log", nil, "pnpm", "pack", "--pack-destination", dest)
	updateManifest(t, app, func(m map[string]any) {
		m["independentSentinel"] = map[string]any{"number": 17, "text": "preserve this manifest value"}
		m["devDependencies"].(map[string]any)["inputs-delivery-sentinel"] = "file:" + filepath.Join(dest, "inputs-delivery-sentinel-1.0.0.tgz")
	})
	write(t, app, "web/patches/independent-sentinel.patch", "diff --git a/index.js b/index.js\n--- a/index.js\n+++ b/index.js\n@@ -1 +1 @@\n-export const sentinel = 'original';\n+export const sentinel = 'preserved independent patch';\n")
	setPatchMap(t, app, "unrelated", map[string]string{"inputs-delivery-sentinel@1.0.0": "patches/independent-sentinel.patch"})
	absoluteSource := filepath.Join(app, "independent-absolute-package")
	write(t, absoluteSource, "package.json", `{"name":"inputs-delivery-absolute-sentinel","version":"1.0.0","type":"module","main":"index.js"}`)
	write(t, absoluteSource, "index.js", "export const sentinel = 'original absolute';\n")
	absDest := filepath.Join(app, "independent-packed-absolute")
	if err := os.MkdirAll(absDest, 0755); err != nil {
		t.Fatal(err)
	}
	must(t, absoluteSource, "pack.log", nil, "pnpm", "pack", "--pack-destination", absDest)
	updateManifest(t, app, func(m map[string]any) {
		m["devDependencies"].(map[string]any)["inputs-delivery-absolute-sentinel"] = "file:" + filepath.Join(absDest, "inputs-delivery-absolute-sentinel-1.0.0.tgz")
	})
	write(t, app, "web/patches/independent-absolute.patch", "diff --git a/index.js b/index.js\n--- a/index.js\n+++ b/index.js\n@@ -1 +1 @@\n-export const sentinel = 'original absolute';\n+export const sentinel = 'preserved absolute patch';\n")
	setPatchMap(t, app, "unrelated-both", map[string]string{"inputs-delivery-sentinel@1.0.0": "patches/independent-sentinel.patch", "inputs-delivery-absolute-sentinel@1.0.0": filepath.Join(app, "web/patches/independent-absolute.patch")})
}

func assertLiteralRawPatchPaths(t *testing.T, app, absoluteOrigin string) {
	t.Helper()
	var document struct {
		Patched map[string]any `yaml:"patchedDependencies"`
	}
	if err := yaml.Unmarshal([]byte(content(t, filepath.Join(app, "web/pnpm-workspace.yaml"))), &document); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"inputs-delivery-sentinel@1.0.0": "patches/independent-sentinel.patch", "inputs-delivery-absolute-sentinel@1.0.0": filepath.Join(absoluteOrigin, "web/patches/independent-absolute.patch"), "@sveltejs/kit@3.0.0": "patches/skgo-kit-3.0.0-queue.patch"} {
		got, ok := document.Patched[key].(string)
		if !ok || got != want {
			t.Fatalf("raw authored patch entry%s=%#v; want literal%s", key, document.Patched[key], want)
		}
	}
}

func authorAliasFlowPatchMap(t *testing.T, app string) {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content(t, filepath.Join(app, "web/pnpm-workspace.yaml"))), &document); err != nil {
		t.Fatal(err)
	}
	root := document.Content[0]
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle, Anchor: "independent_patches"}
	for _, pair := range []struct{ key, value string }{{"inputs-delivery-sentinel@1.0.0", "patches/independent-sentinel.patch"}, {"inputs-delivery-absolute-sentinel@1.0.0", filepath.Join(app, "web/patches/independent-absolute.patch")}} {
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pair.key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pair.value})
	}
	kept := []*yaml.Node{}
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != "patchedDependencies" {
			kept = append(kept, root.Content[index], root.Content[index+1])
		}
	}
	catalogs := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "independentproof"}, mapping}}
	root.Content = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "catalogs"}, catalogs, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "patchedDependencies"}, &yaml.Node{Kind: yaml.AliasNode, Value: "independent_patches", Alias: mapping})
	body, err := yaml.Marshal(&document)
	if err != nil {
		t.Fatal(err)
	}
	var check map[string]any
	if err := yaml.Unmarshal(body, &check); err != nil {
		t.Fatalf("invalid authored alias/flow fixture:%v", err)
	}
	write(t, app, "web/pnpm-workspace.yaml", string(body))
}

func configuredSnapshot(t *testing.T, app string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, name := range []string{"web/package.json", "web/pnpm-workspace.yaml", "web/patches/skgo-kit-3.0.0-queue.patch", "web/patches/independent-sentinel.patch", "web/vite.config.ts", "web/src/routes/+page.svelte"} {
		path := filepath.Join(app, name)
		if b, err := os.ReadFile(path); err == nil {
			result[name] = string(b)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	return result
}

func TestDeliveryExistingConsumerUpgradeApplyCheckAndFrozenReinstall(t *testing.T) {
	p := packSDK(t, candidate(t), filepath.Base(candidate(t)))
	app := filepath.Join(proofRoot, "delivery-fixtures", "existing-upgrade-"+filepath.Base(p.sdk))
	decoy := filepath.Join(proofRoot, "delivery-fixtures/stock-v0.16.4")
	decoyBefore := configuredSnapshot(t, decoy)
	decoyQueue := content(t, filepath.Join(decoy, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js"))
	defer func() {
		for path, want := range decoyBefore {
			if content(t, filepath.Join(decoy, path)) != want {
				t.Errorf("separate stock decoy changed%s", path)
			}
		}
		if content(t, filepath.Join(decoy, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js")) != decoyQueue {
			t.Error("separate stock decoy Kit source changed")
		}
	}()
	authoredCopy(t, filepath.Join(proofRoot, "delivery-fixtures/stock-v0.16.4"), app)
	mod := content(t, filepath.Join(app, "go.mod"))
	mod = strings.ReplaceAll(mod, filepath.Join(proofRoot, "delivery/baseline-sdk"), p.sdk)
	write(t, app, "go.mod", mod)
	must(t, app, "upgrade-sdk-tidy.log", nil, "go", "mod", "tidy")
	updateManifest(t, app, func(m map[string]any) {
		m["devDependencies"].(map[string]any)["@skgo/sveltekit-adapter"] = "file:" + p.adapter
	})
	unrelatedSentinel(t, app)
	authorInputs(t, app, "ts")
	write(t, app, "web/src/routes/sentinel/+page.svelte", "<h1>Preserved authored page17</h1>\n")
	must(t, filepath.Join(app, "web"), "native-upgrade-unpatched.log", nil, "pnpm", "install", "--no-frozen-lockfile")
	const stock = "dbe8bbacab35cbd119d6f5bcf9b527ce0f7a1bfa63954b5ebe047df49f46c6d5"
	if got := sha(content(t, filepath.Join(app, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js"))); got != stock {
		t.Fatalf("upgraded unpatched fixture queue%s want stock%s", got, stock)
	}
	must(t, app, "generate-unpatched.log", nil, "go", "generate", "./...")
	out, err := deliveryRun(t, app, "named-unpatched-build.log", nil, "just", "build")
	if err == nil || !strings.Contains(out, "SKGO_KIT_PRERENDER_QUEUE") {
		t.Fatalf("unpatched Inputs guard expected named nonzero, got%v:%s", err, out)
	}
	authorAliasFlowPatchMap(t, app)
	before := configuredSnapshot(t, app)
	must(t, app, "public-apply.log", nil, "go", "tool", "skgo", "kit-patch", "--web", "web", "--apply")
	out, err = deliveryRun(t, app, "configured-not-installed-check.log", nil, "go", "tool", "skgo", "kit-patch", "--web", "web", "--check")
	if err == nil {
		t.Fatalf("configured stock installation passed check:%s", out)
	}
	if content(t, filepath.Join(app, "web/node_modules/inputs-delivery-sentinel/index.js")) != "export const sentinel = 'preserved independent patch';\n" {
		t.Fatal("unrelated native patch did not apply")
	}
	if content(t, filepath.Join(app, "web/patches/independent-sentinel.patch")) != before["web/patches/independent-sentinel.patch"] || content(t, filepath.Join(app, "web/vite.config.ts")) != before["web/vite.config.ts"] {
		t.Fatal("apply changed unrelated authored configuration")
	}
	must(t, filepath.Join(app, "web"), "native-public-install.log", nil, "node_modules/.bin/vp", "install", "--no-frozen-lockfile")
	verifyInstalled(t, app)
	configured := configuredSnapshot(t, app)
	assertLiteralRawPatchPaths(t, app, app)
	must(t, app, "public-apply-idempotent.log", nil, "go", "tool", "skgo", "kit-patch", "--web", "web", "--apply")
	after := configuredSnapshot(t, app)
	for path, want := range configured {
		if after[path] != want {
			t.Fatalf("repeat apply changed authored file%s", path)
		}
	}
	if !strings.Contains(content(t, filepath.Join(app, "web/package.json")), "preserve this manifest value") || content(t, filepath.Join(app, "web/src/routes/sentinel/+page.svelte")) != "<h1>Preserved authored page17</h1>\n" {
		t.Fatal("manifest/page sentinel lost")
	}
	buildAndAssert(t, app, "ts", "upgraded")
	frozen := frozenReinstall(t, app, "existing-"+filepath.Base(p.sdk))
	assertLiteralRawPatchPaths(t, frozen, app)
	buildAndAssert(t, frozen, "ts", "frozen")
}

func smallTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".svelte-kit" {
				return filepath.SkipDir
			}
			result[rel+"/"] = "directory"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = sha(string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func rejectWithoutWrites(t *testing.T, caller, web, label, diagnostic string, extra ...string) {
	t.Helper()
	rejectWithEnvironmentWithoutWrites(t, caller, web, label, diagnostic, nil, extra...)
}

func rejectWithEnvironmentWithoutWrites(t *testing.T, caller, web, label, diagnostic string, env []string, extra ...string) {
	t.Helper()
	before := smallTree(t, web)
	args := append([]string{"go", "tool", "skgo", "kit-patch", "--web", web, "--apply"}, extra...)
	out, err := deliveryRun(t, caller, "refusal-"+label+".log", env, args...)
	if err == nil || !strings.Contains(out, diagnostic) {
		t.Fatalf("%s expected nonzero%s got%v:%s", label, diagnostic, err, out)
	}
	after := smallTree(t, web)
	if len(after) != len(before) {
		t.Fatalf("%s authored file/directory count changed%d=>%d", label, len(before), len(after))
	}
	for path, want := range before {
		if after[path] != want {
			t.Fatalf("%s changed authored path%s", label, path)
		}
	}
}

func TestDeliveryParentWorkspaceAndConflictRefuseBeforeWrites(t *testing.T) {
	caller := filepath.Join(proofRoot, "delivery-fixtures", "existing-upgrade-"+filepath.Base(candidate(t)))
	fixtureLabel := filepath.Base(candidate(t)) + os.Getenv("SKGO_DELIVERY_CONTROL_SUFFIX")
	for _, explicit := range []bool{false, true} {
		label := fmt.Sprintf("parent-workspace-explicit-%t", explicit)
		t.Run(label, func(t *testing.T) {
			root := filepath.Join(proofRoot, "delivery-fixtures", label+"-"+fixtureLabel)
			write(t, root, "pnpm-workspace.yaml", "packages:\n - web\nonlyBuiltDependencies:\n - independent-proof-preserve17\n")
			write(t, root, "web/package.json", `{"name":"parent-refusal","private":true,"devDependencies":{"@sveltejs/kit":"^3.0.0"}}`)
			before := smallTree(t, root)
			extra := []string{}
			if explicit {
				extra = []string{"--workspace", root}
			}
			rejectWithoutWrites(t, caller, filepath.Join(root, "web"), label, "refusing to modify parent pnpm workspace", extra...)
			after := smallTree(t, root)
			if len(after) != len(before) {
				t.Fatal("parent workspace refusal wrote a file/directory")
			}
			for path, want := range before {
				if after[path] != want {
					t.Fatalf("parent workspace changed%s", path)
				}
			}
		})
	}
	for _, kind := range []string{"different-destination", "incompatible-patch"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(proofRoot, "delivery-fixtures", kind+"-"+fixtureLabel)
			write(t, root, "web/package.json", `{"name":"conflict-refusal","private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"@sveltejs/kit":"^3.0.0"}}`)
			write(t, root, "web/pnpm-workspace.yaml", "onlyBuiltDependencies:\n - independent-proof-preserve17\n")
			want := "refusing to overwrite different patch bytes"
			if kind == "different-destination" {
				write(t, root, "web/patches/skgo-kit-3.0.0-queue.patch", "different authored patch bytes17\n")
			} else {
				want = "conflicting Kit patch entry"
				write(t, root, "web/patches/authored-kit.patch", "preserve conflicting authored patch17\n")
				setPatchMap(t, root, kind, map[string]string{"@sveltejs/kit@^3.0.0": "patches/authored-kit.patch"})
			}
			rejectWithoutWrites(t, caller, filepath.Join(root, "web"), kind, want)
		})
	}
	for _, tc := range []struct{ name, source string }{{"malformed-yaml", "patchedDependencies: [broken\n"}, {"duplicate-yaml", "patchedDependencies: {}\npatchedDependencies: {}\n"}, {"nonstring-yaml-key", "patchedDependencies:\n  17: patches/authored.patch\n"}, {"nonstring-yaml-value", "patchedDependencies:\n  unrelated@1.0.0: false\n"}, {"complex-yaml-key", "patchedDependencies:\n  ? [first, second]\n  : patches/authored.patch\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(proofRoot, "delivery-fixtures", tc.name+"-"+fixtureLabel)
			write(t, root, "web/package.json", `{"name":"yaml-refusal","private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"@sveltejs/kit":"^3.0.0"}}`)
			write(t, root, "web/pnpm-workspace.yaml", tc.source)
			rejectWithoutWrites(t, caller, filepath.Join(root, "web"), tc.name, "workspace")
		})
	}
	t.Run("environment-ambiguity", func(t *testing.T) {
		root := filepath.Join(proofRoot, "delivery-fixtures", "environment-ambiguity-"+fixtureLabel)
		web := filepath.Join(root, "web")
		write(t, root, "web/package.json", `{"name":"environment-refusal","private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"@sveltejs/kit":"^3.0.0"}}`)
		write(t, root, "web/pnpm-workspace.yaml", "patchedDependencies:\n  unrelated@1.0.0: patches/raw.patch\n")
		write(t, root, "web/patches/raw.patch", "preserve authored raw patch bytes17\n")
		env := []string{`PNPM_CONFIG_PATCHED_DEPENDENCIES={"other@1.0.0":"patches/environment.patch"}`}
		before := smallTree(t, web)
		out := must(t, web, "../native-environment-getter.log", env, "pnpm", "--pm-on-fail=ignore", "config", "get", "patchedDependencies", "--location=project", "--json")
		var effective map[string]string
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &effective); err != nil {
			t.Fatalf("actual environment getter:%v:%s", err, out)
		}
		if len(effective) != 1 || effective["other@1.0.0"] != filepath.Join(web, "patches/environment.patch") {
			t.Fatalf("actual native environment getter%v; expected independently authored other key/path", effective)
		}
		after := smallTree(t, web)
		if len(after) != len(before) {
			t.Fatal("flagged native ambiguity read created an authored path")
		}
		for path, want := range before {
			if after[path] != want {
				t.Fatalf("native ambiguity read changed%s", path)
			}
		}
		rejectWithEnvironmentWithoutWrites(t, caller, web, "environment-ambiguity", "ambiguous", env)
	})
}

func TestDeliveryUnsupportedInstalledSourceRefusesBeforeWrites(t *testing.T) {
	sdk := candidate(t)
	caller := filepath.Join(proofRoot, "delivery-fixtures", "existing-upgrade-"+filepath.Base(sdk))
	stock, err := filepath.EvalSymlinks(filepath.Join(proofRoot, "delivery-fixtures/stock-v0.16.4/web/node_modules/@sveltejs/kit"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"unsupported-version", "altered-source"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(proofRoot, "delivery-fixtures", kind+"-"+filepath.Base(sdk))
			source := filepath.Join(root, "pre-install-kit")
			authoredCopy(t, stock, source)
			var pkg map[string]any
			if err := json.Unmarshal([]byte(content(t, filepath.Join(source, "package.json"))), &pkg); err != nil {
				t.Fatal(err)
			}
			diagnostic := "installed Kit is @sveltejs/kit@3.0.1"
			if kind == "unsupported-version" {
				pkg["version"] = "3.0.1"
				b, _ := json.MarshalIndent(pkg, "", "  ")
				write(t, source, "package.json", string(b)+"\n")
			} else {
				diagnostic = "queue source has unexpected SHA-256"
				queue := content(t, filepath.Join(source, "src/core/postbuild/queue.js"))
				write(t, source, "src/core/postbuild/queue.js", queue+"\n// independent pre-install altered source control\n")
			}
			tarDir := filepath.Join(root, "packed-kit")
			if err := os.MkdirAll(tarDir, 0755); err != nil {
				t.Fatal(err)
			}
			must(t, source, "native-pack.log", nil, "pnpm", "pack", "--pack-destination", tarDir)
			files, err := filepath.Glob(filepath.Join(tarDir, "*.tgz"))
			if err != nil || len(files) != 1 {
				t.Fatalf("control tarball%v %v", files, err)
			}
			body, _ := json.Marshal(map[string]any{"name": "kit-source-control", "private": true, "packageManager": "pnpm@12.9.1", "devDependencies": map[string]string{"@sveltejs/kit": "file:" + files[0]}})
			write(t, root, "web/package.json", string(body)+"\n")
			write(t, root, "web/pnpm-workspace.yaml", "onlyBuiltDependencies:\n - independent-proof-preserve17\n")
			must(t, filepath.Join(root, "web"), "native-install-control.log", nil, "pnpm", "install", "--no-frozen-lockfile")
			queuePath := filepath.Join(root, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js")
			before := content(t, queuePath)
			rejectWithoutWrites(t, caller, filepath.Join(root, "web"), kind, diagnostic)
			if content(t, queuePath) != before {
				t.Fatal("public helper changed installed native queue source")
			}
		})
	}
}

func TestDeliveryRemovedOwnedPatchRestoresNamedStockFailure(t *testing.T) {
	sdk := candidate(t)
	root := filepath.Join(proofRoot, "delivery-fixtures", "removal-control-"+filepath.Base(sdk))
	authoredCopy(t, filepath.Join(proofRoot, "delivery-fixtures", "existing-upgrade-"+filepath.Base(sdk)), root)
	setPatchMap(t, root, "remove-owned-only", map[string]string{"inputs-delivery-sentinel@1.0.0": "patches/independent-sentinel.patch", "inputs-delivery-absolute-sentinel@1.0.0": filepath.Join(proofRoot, "delivery-fixtures", "existing-upgrade-"+filepath.Base(sdk), "web/patches/independent-absolute.patch")})
	if err := os.Remove(filepath.Join(root, "web/patches/skgo-kit-3.0.0-queue.patch")); err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(root, "web"), "native-stock-reinstall.log", nil, "pnpm", "install", "--no-frozen-lockfile")
	const stock = "dbe8bbacab35cbd119d6f5bcf9b527ce0f7a1bfa63954b5ebe047df49f46c6d5"
	if got := sha(content(t, filepath.Join(root, "web/node_modules/@sveltejs/kit/src/core/postbuild/queue.js"))); got != stock {
		t.Fatalf("removal native install hash%s want stock%s", got, stock)
	}
	trace := filepath.Join(root, "removal-build-calls.log")
	out, err := deliveryRun(t, root, "removed-only-owned-patch-build.log", []string{"SKGO_INPUTS_RECEIPT=" + trace}, "just", "build")
	if err == nil || !strings.Contains(out, "SKGO_KIT_PRERENDER_QUEUE") {
		t.Fatalf("removal expected original minimal build to fail named guard: %v:%s", err, out)
	}
	if _, err := os.Stat(trace); !os.IsNotExist(err) {
		t.Fatalf("guard allowed Go Inputs/body IPC after removal:%v", err)
	}
	if content(t, filepath.Join(root, "web/node_modules/inputs-delivery-sentinel/index.js")) != "export const sentinel = 'preserved independent patch';\n" {
		t.Fatal("removal lost unrelated native patch")
	}
}

func TestDeliveryCorrectedNativeDelayedAsyncInputs(t *testing.T) {
	sdk := candidate(t)
	app := filepath.Join(proofRoot, "delivery-fixtures", "corrected-native-async-"+filepath.Base(sdk))
	authoredCopy(t, filepath.Join(proofRoot, "delivery-fixtures", "fresh-ts-"+filepath.Base(sdk)), app)
	if err := os.RemoveAll(filepath.Join(app, "web/src")); err != nil {
		t.Fatal(err)
	}
	stock := filepath.Join(proofRoot, "native-fixtures/async-queue-closed")
	if err := filepath.WalkDir(filepath.Join(stock, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(stock, path)
		write(t, app, "web/"+rel, content(t, path))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	config := content(t, filepath.Join(stock, "vite.config.ts"))
	config = strings.Replace(config, "http://127.0.0.1:19341", "https://prerender-proof.invalid", 1)
	write(t, app, "web/vite.config.ts", config)
	must(t, filepath.Join(app, "web"), "native-frozen-install.log", nil, "pnpm", "install", "--frozen-lockfile")
	verifyInstalled(t, app)
	must(t, filepath.Join(app, "web"), "corrected-native-build.log", nil, "node_modules/.bin/vp", "build")
	trace := content(t, filepath.Join(app, "web/async.log"))
	if trace != "inputs:start\ninputs:end\nbody:atlas\nbody:beacon\n" {
		t.Fatalf("corrected native async producer/body literal receipts:%q", trace)
	}
	for _, tc := range []struct{ key, value string }{{"WyJhdGxhcyJd", "atlas"}, {"WyJiZWFjb24iXQ", "beacon"}} {
		path := filepath.Join(app, "web/.svelte-kit/output/prerendered/data/_app/remote/3215r6/catalog", tc.key)
		got := content(t, path)
		want := fmt.Sprintf(`{"type":"result","data":"[{\"_\":1,\"p\":2},\"build:%s\",{\"3215r6/catalog/%s\":3},{\"v\":1}]"}`, tc.value, tc.key)
		if got != want {
			t.Fatalf("corrected native literal artifact%s got%s want%s", tc.key, got, want)
		}
	}
	if !strings.Contains(content(t, filepath.Join(app, "web/.svelte-kit/output/prerendered/pages/index.html")), "Native inputs fixture") {
		t.Fatal("corrected native initial page missing authored text")
	}
}

func TestDeliveryNativeCorrectionFromCommittedPayload(t *testing.T) {
	root := filepath.Join(proofRoot, "delivery-fixtures/native-correction-A0ab6303")
	if _, err := os.Stat(root); err == nil {
		t.Fatalf("refusing to overwrite retained native fixture%s", root)
	}
	stock := filepath.Join(proofRoot, "native-fixtures/async-queue-closed")
	if err := filepath.WalkDir(filepath.Join(stock, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(stock, path)
		write(t, root, rel, content(t, path))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var pkg map[string]any
	if err := json.Unmarshal([]byte(content(t, filepath.Join(stock, "package.json"))), &pkg); err != nil {
		t.Fatal(err)
	}
	pkg["packageManager"] = "pnpm@12.9.1"
	pkg["dependencies"] = map[string]string{"@sveltejs/kit": "3.0.0", "@sveltejs/vite-plugin-svelte": "7.3.1", "svelte": "5.57.1", "typescript": "6.0.3", "vite-plus": "1.0.0", "vite": "npm:@voidzero-dev/vite-plus-core@1.0.0"}
	body, _ := json.MarshalIndent(pkg, "", "  ")
	write(t, root, "package.json", string(body)+"\n")
	config := strings.Replace(content(t, filepath.Join(stock, "vite.config.ts")), "http://127.0.0.1:19341", "https://prerender-proof.invalid", 1)
	write(t, root, "vite.config.ts", config)
	patch := content(t, filepath.Join(proofRoot, "delivery/queue-sdk-0ab6303/internal/adapter/skgo-adapter/compat/kit-3.0.0-queue.patch"))
	if sha(patch) != "0d35d370d3e5fb6e0c801cd1079013b3d487d6e301e27777f26fabbf6e0106c4" {
		t.Fatal("committed native patch canonical hash mismatch")
	}
	write(t, root, "patches/skgo-kit-3.0.0-queue.patch", patch)
	must(t, root, "native-config.log", nil, "pnpm", "config", "set", "patchedDependencies", `{"@sveltejs/kit@3.0.0":"patches/skgo-kit-3.0.0-queue.patch"}`, "--location=project", "--json")
	must(t, root, "native-install.log", nil, "pnpm", "install", "--no-frozen-lockfile", "--store-dir", filepath.Join(proofRoot, "delivery/native-correction-store"))
	if got := sha(content(t, filepath.Join(root, "node_modules/@sveltejs/kit/src/core/postbuild/queue.js"))); got != "400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5" {
		t.Fatalf("native-installed corrected queue hash%s", got)
	}
	must(t, root, "corrected-native-build.log", nil, "node_modules/.bin/vp", "build")
	if got := content(t, filepath.Join(root, "async.log")); got != "inputs:start\ninputs:end\nbody:atlas\nbody:beacon\n" {
		t.Fatalf("corrected native exact producer/body trace%q", got)
	}
	for _, tc := range []struct{ key, value string }{{"WyJhdGxhcyJd", "atlas"}, {"WyJiZWFjb24iXQ", "beacon"}} {
		got := content(t, filepath.Join(root, ".svelte-kit/output/prerendered/data/_app/remote/3215r6/catalog", tc.key))
		want := fmt.Sprintf(`{"type":"result","data":"[{\"_\":1,\"p\":2},\"build:%s\",{\"3215r6/catalog/%s\":3},{\"v\":1}]"}`, tc.value, tc.key)
		if got != want {
			t.Fatalf("corrected native exact artifact%s got%s want%s", tc.key, got, want)
		}
	}
	if !strings.Contains(content(t, filepath.Join(root, ".svelte-kit/output/prerendered/pages/index.html")), "Native inputs fixture") {
		t.Fatal("corrected native initial page missing authored text")
	}
}

func TestDeliveryNativeUniversalImportNeedsNoClientID(t *testing.T) {
	for _, noarg := range []bool{false, true} {
		t.Run(fmt.Sprintf("noarg-%t", noarg), func(t *testing.T) {
			root := filepath.Join(proofRoot, "delivery-fixtures", fmt.Sprintf("native-universal-caller-free-A0ab6303-noarg-%t", noarg))
			authoredCopy(t, filepath.Join(proofRoot, "delivery-fixtures/native-correction-A0ab6303"), root)
			if err := os.Remove(filepath.Join(root, "src/routes/+layout.server.ts")); err != nil {
				t.Fatal(err)
			}
			write(t, root, "src/routes/+layout.ts", `import '../lib/fixture.remote';export const prerender=true;`)
			if noarg {
				write(t, root, "src/lib/fixture.remote.ts", `import {prerender} from '$app/server';import {appendFileSync} from 'node:fs';export const empty=prerender(()=>{appendFileSync('async.log','body:empty\n');return 'build:empty';},{inputs:async()=>{appendFileSync('async.log','inputs:empty\n');await new Promise(resolve=>setTimeout(resolve,500));return [];}});`)
			}
			if err := os.Remove(filepath.Join(root, "async.log")); err != nil {
				t.Fatal(err)
			}
			must(t, root, "native-frozen-install.log", nil, "pnpm", "install", "--frozen-lockfile", "--store-dir", filepath.Join(proofRoot, "delivery/native-correction-store"))
			must(t, root, "native-universal-build.log", nil, "node_modules/.bin/vp", "build")
			name := "catalog"
			wantTrace := "inputs:start\ninputs:end\nbody:atlas\nbody:beacon\n"
			cases := []struct{ suffix, key, value string }{{"catalog/WyJhdGxhcyJd", "catalog/WyJhdGxhcyJd", "build:atlas"}, {"catalog/WyJiZWFjb24iXQ", "catalog/WyJiZWFjb24iXQ", "build:beacon"}}
			if noarg {
				name = "empty"
				wantTrace = "inputs:empty\nbody:empty\n"
				cases = []struct{ suffix, key, value string }{{"empty", "empty/", "build:empty"}}
			}
			if got := content(t, filepath.Join(root, "async.log")); got != wantTrace {
				t.Fatalf("native caller-free trace%q want%q", got, wantTrace)
			}
			for _, tc := range cases {
				got := content(t, filepath.Join(root, ".svelte-kit/output/prerendered/data/_app/remote/3215r6", tc.suffix))
				want := fmt.Sprintf(`{"type":"result","data":"[{\"_\":1,\"p\":2},\"%s\",{\"3215r6/%s\":3},{\"v\":1}]"}`, tc.value, tc.key)
				if got != want {
					t.Fatalf("native caller-free artifact%s got%s want%s", tc.suffix, got, want)
				}
			}
			if !strings.Contains(content(t, filepath.Join(root, ".svelte-kit/output/prerendered/pages/index.html")), "Native inputs fixture") {
				t.Fatal("positive authored page missing")
			}
			clientIDs := 0
			jsFiles := 0
			err := filepath.WalkDir(filepath.Join(root, ".svelte-kit/output/client"), func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || filepath.Ext(path) != ".js" {
					return nil
				}
				jsFiles++
				clientIDs += strings.Count(content(t, path), "3215r6/"+name)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if jsFiles == 0 || clientIDs != 0 {
				t.Fatalf("native caller-free client graph files%d exact remoteIDs%d want literal0", jsFiles, clientIDs)
			}
			t.Logf("actual native universal import/no component caller: clientJSfiles%d exact3215r6/%s IDs0; literal native artifacts and authored root present", jsFiles, name)
		})
	}
}
