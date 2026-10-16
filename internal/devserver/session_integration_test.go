//go:build skgo_dev_integration

package devserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// This opt-in contract runs real Vite and native application children. Missing
// prerequisites are failures, never silently skipped qualification.
func TestRealSessionNativeRecoveryAndIsolation(t *testing.T) {
	if _, err := exec.LookPath("vp"); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "example/web/node_modules/@sveltejs/kit/package.json")); err != nil {
		t.Fatal(err)
	}
	sharedTypes := filepath.Join(root, "example/web/node_modules/$app/types/index.d.ts")
	originalTypes, err := os.ReadFile(sharedTypes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(sharedTypes)
		if err != nil || !bytes.Equal(after, originalTypes) {
			t.Errorf("session fixture changed shared Kit types: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	type session struct {
		origin string
		log    *sessionLog
		done   chan error
		cancel context.CancelFunc
		app    string
	}
	start := func(label string) *session {
		t.Helper()
		app := filepath.Join(t.TempDir(), "app")
		// Durable application state lives outside captured compiler inputs.
		projectDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(projectDir, "catalog"), []byte("saved-project-340"), 0600); err != nil {
			t.Fatal(err)
		}
		write := func(name, body string) {
			t.Helper()
			p := filepath.Join(app, name)
			if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(p, []byte(body), 0644); e != nil {
				t.Fatal(e)
			}
		}
		mod, e := os.ReadFile(filepath.Join(root, "example/go.mod"))
		if e != nil {
			t.Fatal(e)
		}
		write("go.mod", strings.ReplaceAll(strings.ReplaceAll(string(mod), "module github.com/tylergannon/skgo/example", "module example.com/sessionfixture"), "=> ../", "=> "+filepath.ToSlash(root)))
		sums, e := os.ReadFile(filepath.Join(root, "example/go.sum"))
		if e != nil {
			t.Fatal(e)
		}
		write("go.sum", string(sums))
		pkg, e := os.ReadFile(filepath.Join(root, "example/web/package.json"))
		if e != nil {
			t.Fatal(e)
		}
		write("web/package.json", string(pkg))
		dependencies := filepath.Join(root, "example/web/node_modules")
		modules := filepath.Join(app, "web/node_modules")
		if e = os.MkdirAll(modules, 0755); e != nil {
			t.Fatal(e)
		}
		entries, e := os.ReadDir(dependencies)
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "$app" || strings.HasPrefix(name, "$app.") || name == ".cache" || strings.HasPrefix(name, ".vite") {
				continue
			}
			if e = os.Symlink(filepath.Join(dependencies, name), filepath.Join(modules, name)); e != nil {
				t.Fatal(e)
			}
		}
		write("web/vite.config.ts", sessionConfig("", "_app", "https://initial.test"))
		write("web/src/app.html", `<!doctype html><html><head>%sveltekit.head%</head><body><div>%sveltekit.body%</div></body></html>`)
		write("web/src/routes/+page.svelte", `<script>import{PUBLIC_RECEIPT}from'$app/env/public';</script><h1>literal session frontend {PUBLIC_RECEIPT}</h1>`)
		write("web/.env.development", "PUBLIC_RECEIPT=ENV_101\n")
		write("web/src/env.go", "package appenv\nimport s \"github.com/tylergannon/skgo\"\ntype Config struct { Receipt string `env:\"PUBLIC_RECEIPT,public,static\"` }\nvar _ = s.Environment[Config]()\n")
		write("locals/locals.go", "package locals\ntype Locals struct{}\n")
		write("generated/config.go", "package generated\n//go:generate go tool skgo generate --web ../web --locals-package example.com/sessionfixture/locals --locals-type Locals\n")
		write("web/src/routes/orders/server.go", sessionRoute("4101", 7))
		write("server.go", fmt.Sprintf(`package sessionfixture
import("io/fs";"net/http";"os";"strconv";"strings";"sync";"syscall";"errors";appenv "example.com/sessionfixture/web/src";"github.com/tylergannon/skgo"; generated "example.com/sessionfixture/generated")
func NewHandler(dist fs.FS,origin string)(http.Handler,error){return newHandler(dist,origin,nil)}
func NewDevelopmentHandler(dist fs.FS,origin string,native *skgo.NativeDevelopment)(http.Handler,error){return newHandler(dist,origin,native)}
var rejectStartup bool
var projectOwner=sync.OnceValues(func()(*os.File,error){
 lock,err:=os.OpenFile(%q,os.O_CREATE|os.O_RDWR,0600);if err!=nil{return nil,err}
 if err:=syscall.Flock(int(lock.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{lock.Close();return nil,err}
 return lock,nil
})
func newHandler(dist fs.FS,origin string,native *skgo.NativeDevelopment)(http.Handler,error){
 if _,err:=projectOwner();err!=nil{return nil,err}
 if rejectStartup{return nil,errors.New("literal startup rejection 340")}
 durable,err:=os.ReadFile(%q);if err!=nil{return nil,err}
 manifest,err:=skgo.ReadManifest(dist);if err!=nil{return nil,err}
 remotes,err:=skgo.NewRemotes(manifest.RemoteConfig(origin),generated.Remotes()...);if err!=nil{return nil,err}
 loads,err:=skgo.NewLoads(manifest.LoadConfig(origin),generated.Loads()...);if err!=nil{return nil,err}
 actions,err:=skgo.NewActions(generated.Actions()...);if err!=nil{return nil,err}
 envValues,envSnapshot,err:=skgo.LoadEnvironment[appenv.Config](dist,os.LookupEnv);if err!=nil{return nil,err}
 var renderer *skgo.SSR;var pages http.Handler
 options:=skgo.SSROptions{Actions:actions,Environment:envSnapshot}
 if native==nil{renderer,err=skgo.NewSSR(dist,manifest,loads,remotes,options)}else{renderer,err=native.NewSSR(dist,manifest,loads,remotes,options)};if err!=nil{return nil,err}
 if native==nil{pages,err=skgo.NewStaticHandler(dist,skgo.WithSSR(renderer))}else{pages,err=native.NewStaticHandler(dist,skgo.WithSSR(renderer))};if err!=nil{return nil,err}
 endpoints,err:=skgo.NewEndpoints(manifest.EndpointConfig(origin),generated.Endpoints()...);if err!=nil{return nil,err}
 cfg:=manifest.HandleConfig();cfg.Origin=origin;cfg.Loads=loads
 app:=generated.RequestBoundary(cfg,loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Project-State",string(durable));w.Header().Set("X-Go-Pid",strconv.Itoa(os.Getpid()));w.Header().Set("X-Session",%q);w.Header().Set("X-Origin",origin);w.Header().Set("X-Environment",envValues.Receipt);w.Header().Set("X-App-Dir",manifest.AppDir);w.Header().Set("X-Trusted-Origins",strings.Join(manifest.TrustedOrigins,","));app.ServeHTTP(w,r)}),nil
}
`, filepath.Join(projectDir, "owner.lock"), filepath.Join(projectDir, "catalog"), label))
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		address := ln.Addr().String()
		occupied := label == "ALPHA"
		if occupied {
			t.Cleanup(func() { ln.Close() })
		} else {
			ln.Close()
		}
		childCtx, stop := context.WithCancel(ctx)
		s := &session{origin: "http://" + address, log: &sessionLog{}, done: make(chan error, 1), cancel: stop, app: app}
		go func() {
			s.done <- Run(childCtx, Options{Root: app, Listen: address, StrictPort: !occupied, Output: s.log})
		}()
		t.Cleanup(func() {
			stop()
			select {
			case e := <-s.done:
				if e != nil && ctx.Err() == nil {
					t.Errorf("session shutdown: %v\n%s", e, s.log.String())
				}
			case <-time.After(15 * time.Second):
				t.Error("session did not stop")
			}
		})

		if occupied {
			go func() {
				deadline := time.Now().Add(60 * time.Second)
				for time.Now().Before(deadline) {
					if strings.Contains(s.log.String(), "building native application host") {
						if err := os.WriteFile(filepath.Join(app, "web/src/routes/orders/server.go"), []byte(sessionRoute("4101", 8)), 0644); err != nil {
							s.log.Write([]byte(err.Error()))
						}
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
			}()
			deadline := time.Now().Add(75 * time.Second)
			for time.Now().Before(deadline) {
				match := regexp.MustCompile(`skgo dev: (http://[^ ]+) \(Go pid`).FindStringSubmatch(s.log.String())
				if len(match) > 1 {
					s.origin = match[1]
					break
				}
				select {
				case e := <-s.done:
					s.done <- e
					t.Fatalf("occupied-port startup: %v\n%s", e, s.log.String())
				default:
				}
				time.Sleep(50 * time.Millisecond)
			}
			if s.origin == "http://"+address {
				t.Fatalf("did not discover a different public origin\n%s", s.log.String())
			}

		}
		return s
	}
	check := func(s *session, label, body string) {
		t.Helper()
		deadline := time.Now().Add(75 * time.Second)
		reported := false
		for time.Now().Before(deadline) {
			r, e := http.Get(s.origin + "/orders")
			if e == nil {
				b, _ := io.ReadAll(r.Body)
				r.Body.Close()
				if r.StatusCode >= 500 && !reported {
					t.Logf("initial upstream failure status=%d body=%s\n%s", r.StatusCode, b, s.log.String())
					reported = true
				}
				if r.StatusCode == 200 && string(b) == body && r.Header.Get("X-Session") == label && r.Header.Get("X-Origin") == s.origin && r.Header.Get("X-Project-State") == "saved-project-340" {
					return
				}
			}
			select {
			case e := <-s.done:
				s.done <- e
				t.Fatalf("session exited: %v\n%s", e, s.log.String())
			default:
			}
			time.Sleep(80 * time.Millisecond)
		}
		t.Fatalf("want %s %s at %s\n%s", label, body, s.origin, s.log.String())
	}
	alpha := start("ALPHA")
	check(alpha, "ALPHA", "literal-4101:8")
	beta := start("BETA")
	check(beta, "BETA", "literal-4101:7")
	pid := alpha.log.lastPID()
	if pid == 0 {
		t.Fatalf("missing actual host PID: %s", alpha.log.String())
	}
	route := filepath.Join(alpha.app, "web/src/routes/orders/server.go")
	if e := os.WriteFile(route, []byte(sessionRoute("5202", 8)), 0644); e != nil {
		t.Fatal(e)
	}
	check(alpha, "ALPHA", "literal-5202:8")
	check(beta, "BETA", "literal-4101:7")
	if next := alpha.log.lastPID(); next != pid {
		t.Fatalf("body edit replaced host %d -> %d\n%s", pid, next, alpha.log.String())
	}
	if e := os.WriteFile(route, []byte(sessionRoute("6303", 9)), 0644); e != nil {
		t.Fatal(e)
	}
	check(alpha, "ALPHA", "literal-6303:9")
	deadline := time.Now().Add(10 * time.Second)
	for alpha.log.lastPID() == pid && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	replacement := alpha.log.lastPID()
	if replacement == pid {
		t.Fatalf("initializer change did not replace host\n%s", alpha.log.String())
	}
	if e := syscall.Kill(replacement, syscall.SIGKILL); e != nil {
		t.Fatal(e)
	}
	check(alpha, "ALPHA", "literal-6303:9")
	check(beta, "BETA", "literal-4101:7")
	deadline = time.Now().Add(30 * time.Second)
	for alpha.log.lastPID() == replacement && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if alpha.log.lastPID() == replacement {
		t.Fatalf("Go crash not recovered\n%s", alpha.log.String())
	}

	beforeCombined := alpha.log.lastPID()
	if e := os.WriteFile(route, []byte(sessionRoute("7407", 9)), 0644); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(alpha.app, "web/src/routes/+page.svelte"), []byte(`<script>import{PUBLIC_RECEIPT}from'$app/env/public';</script><h1>literal combined frontend TWO {PUBLIC_RECEIPT}</h1>`), 0644); e != nil {
		t.Fatal(e)
	}
	check(alpha, "ALPHA", "literal-7407:9")
	if alpha.log.lastPID() != beforeCombined {
		t.Fatalf("compatible Go body + Svelte edit replaced host\n%s", alpha.log.String())
	}
	res, err := http.Get(alpha.origin + "/")
	if err != nil {
		t.Fatal(err)
	}
	markup, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(markup), "literal combined frontend TWO") {
		t.Fatalf("native Kit SSR status=%d markup=%s", res.StatusCode, markup)
	}

	check(alpha, "ALPHA", "literal-7407:9")

	// A bad Go edit retains the running application, reports a real compiler
	// diagnostic, and a repaired body can still take the native fast path.
	beforeError := alpha.log.lastPID()
	if err := os.WriteFile(route, []byte("package orders\nthis is invalid Go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for !strings.Contains(alpha.log.String(), "expected declaration") && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(alpha.log.String(), "expected declaration") {
		t.Fatalf("missing compiler diagnostic\n%s", alpha.log.String())
	}
	check(alpha, "ALPHA", "literal-7407:9")
	if err := os.WriteFile(route, []byte(sessionRoute("8508", 9)), 0644); err != nil {
		t.Fatal(err)
	}
	check(alpha, "ALPHA", "literal-8508:9")
	if alpha.log.lastPID() != beforeError {
		t.Fatalf("repair of compatible body replaced host\n%s", alpha.log.String())
	}

	// A declaration change can require a full rebuild before type checking.
	// Give the live application state that a restart cannot recreate, then
	// require every request through the failed build to retain that state/PID.
	stateResponse, err := http.Get(alpha.origin + "/orders?initial=73")
	if err != nil {
		t.Fatal(err)
	}
	stateBody, _ := io.ReadAll(stateResponse.Body)
	stateResponse.Body.Close()
	if stateResponse.StatusCode != 200 || string(stateBody) != "literal-8508:73" {
		t.Fatalf("set live state: status=%d body=%s", stateResponse.StatusCode, stateBody)
	}
	healthyPID := stateResponse.Header.Get("X-Go-Pid")
	if healthyPID != strconv.Itoa(beforeError) {
		t.Fatalf("live Go PID=%q, want %d", healthyPID, beforeError)
	}
	logStart := len(alpha.log.String())
	applicationSource := filepath.Join(alpha.app, "server.go")
	healthySource, err := os.ReadFile(applicationSource)
	if err != nil {
		t.Fatal(err)
	}
	// Route packages are type-checked during generation. An ordinary helper
	// in the application constructor package reaches full host compilation.
	if err := os.WriteFile(applicationSource, append(healthySource, []byte("func brokenDeclaration() int { return \"type-invalid\" }\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(30 * time.Second)
	diagnostic := false
	var observedUntil time.Time
	for time.Now().Before(deadline) {
		res, err := http.Get(alpha.origin + "/orders")
		if err != nil {
			t.Fatalf("failed structural edit interrupted healthy application: %v\n%s", err, alpha.log.String())
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || string(body) != "literal-8508:73" || res.Header.Get("X-Go-Pid") != healthyPID {
			t.Fatalf("failed structural edit lost healthy application: status=%d body=%s PID=%s, want 200 literal-8508:73 PID=%s\n%s", res.StatusCode, body, res.Header.Get("X-Go-Pid"), healthyPID, alpha.log.String())
		}
		if !diagnostic && strings.Contains(alpha.log.String()[logStart:], `cannot use "type-invalid"`) {
			diagnostic = true
			observedUntil = time.Now().Add(time.Second)
		}
		if diagnostic && time.Now().After(observedUntil) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !diagnostic || !strings.Contains(alpha.log.String()[logStart:], "rebuilding revision") {
		t.Fatalf("structural edit did not reach rebuild/type diagnostic\n%s", alpha.log.String())
	}
	// Repairing the declaration must replace the host and restore its initial
	// state, proving the failed candidate did not wedge subsequent rebuilds.
	if err := os.WriteFile(applicationSource, append(healthySource, []byte("func brokenDeclaration() int { return 10 }\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	check(alpha, "ALPHA", "literal-8508:9")
	deadline = time.Now().Add(10 * time.Second)
	for alpha.log.lastPID() == beforeError && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if alpha.log.lastPID() == beforeError {
		t.Fatalf("valid structural repair did not replace host\n%s", alpha.log.String())
	}

	// Recovery must retain admitted native edits newer than the last executable.
	if err := os.WriteFile(route, []byte(sessionRoute("8754", 9)), 0644); err != nil {
		t.Fatal(err)
	}
	check(alpha, "ALPHA", "literal-8754:9")

	// A candidate can compile but reject initialization. It must release its
	// project lock and restore the previous captured inputs rather than strand service.
	goodApplication := append(append([]byte(nil), healthySource...), []byte("func brokenDeclaration() int { return 10 }\n")...)
	logStart = len(alpha.log.String())
	rejected := append(append([]byte(nil), goodApplication...), []byte("func init() { rejectStartup = true }\n")...)
	if err := os.WriteFile(applicationSource, rejected, 0644); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(alpha.log.String()[logStart:], "restored previous application") {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(alpha.log.String()[logStart:], "restored previous application") || !strings.Contains(alpha.log.String()[logStart:], "literal startup rejection 340") {
		t.Fatalf("failed initialization did not restore previous executable\n%s", alpha.log.String())
	}
	check(alpha, "ALPHA", "literal-8754:9")
	// A subsequent valid edit must still recover through the normal rebuild path.
	logStart = len(alpha.log.String())
	goodApplication = []byte(strings.Replace(string(goodApplication), "return 10", "return 11", 1))
	if err := os.WriteFile(applicationSource, goodApplication, 0644); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(alpha.log.String()[logStart:], "rebuild true") {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(alpha.log.String()[logStart:], "rebuild true") {
		t.Fatalf("repair after rejected initialization did not rebuild\n%s", alpha.log.String())
	}
	check(alpha, "ALPHA", "literal-8754:9")

	// Recovering Vite keeps the Go process and stable public origin.
	vitePID := alpha.log.vitePID()
	if vitePID == 0 {
		t.Fatalf("missing Vite child PID\n%s", alpha.log.String())
	}
	if e := syscall.Kill(-vitePID, syscall.SIGKILL); e != nil {
		t.Fatal(e)
	}
	deadline = time.Now().Add(45 * time.Second)
	for alpha.log.vitePID() == vitePID && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if alpha.log.vitePID() == vitePID {
		t.Fatalf("Vite not restarted\n%s", alpha.log.String())
	}

	check(alpha, "ALPHA", "literal-8754:9")
	check(beta, "BETA", "literal-4101:7")

	// Real Vite restarts must publish changed constructor inputs, not just a
	// fresh frontend graph. Independent literals exercise both sides of the seam.
	request := func(path string, status int, body, environment, appDir, trusted string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		var actual string
		for time.Now().Before(deadline) {
			res, err := http.Get(alpha.origin + path)
			if err == nil {
				b, _ := io.ReadAll(res.Body)
				res.Body.Close()
				actual = fmt.Sprintf("status=%d headers=%v body=%s", res.StatusCode, res.Header, b)
				if res.StatusCode == status && strings.Contains(string(b), body) &&
					(environment == "" || (res.Header.Get("X-Environment") == environment && res.Header.Get("X-Origin") == alpha.origin && res.Header.Get("X-Session") == "ALPHA")) &&
					(appDir == "" || res.Header.Get("X-App-Dir") == appDir) &&
					(trusted == "" || res.Header.Get("X-Trusted-Origins") == trusted) {
					return
				}
			}
			time.Sleep(80 * time.Millisecond)
		}
		t.Fatalf("configuration never became current at %s: %s\n%s", path, actual, alpha.log.String())
	}
	request("/orders", 200, "literal-8754:9", "ENV_101", "_app", "https://initial.test")
	request("/", 200, "literal combined frontend TWO ENV_101", "", "", "")
	pid = alpha.log.lastPID()
	if err := os.WriteFile(filepath.Join(alpha.app, "web/vite.config.ts"), []byte(sessionConfig("/base", "assets", "https://updated.test")), 0644); err != nil {
		t.Fatal(err)
	}
	request("/base/orders", 200, "literal-8754:9", "ENV_101", "assets", "https://updated.test")
	request("/base/assets/remote/missing", 200, `"type":"error"`, "ENV_101", "assets", "https://updated.test")
	if alpha.log.lastPID() != pid {
		t.Fatalf("configuration edit unnecessarily replaced Go host\n%s", alpha.log.String())
	}
	if err := os.WriteFile(filepath.Join(alpha.app, "web/.env.development"), []byte("PUBLIC_RECEIPT=ENV_202\n"), 0644); err != nil {
		t.Fatal(err)
	}
	request("/base/orders", 200, "literal-8754:9", "ENV_202", "assets", "https://updated.test")
	request("/base/", 200, "literal combined frontend TWO ENV_202", "", "", "")
	if alpha.log.lastPID() != pid {
		t.Fatalf("dotenv edit unnecessarily replaced Go host\n%s", alpha.log.String())
	}
	// A subsequent compatible body edit still uses the retained native path.
	if err := os.WriteFile(route, []byte(sessionRoute("9609", 9)), 0644); err != nil {
		t.Fatal(err)
	}
	request("/base/orders", 200, "literal-9609:9", "ENV_202", "assets", "https://updated.test")
	if alpha.log.lastPID() != pid {
		t.Fatalf("body edit after config reload replaced Go host\n%s", alpha.log.String())
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	request("/base/orders", 200, "literal-9609:9", "ENV_202", "assets", "https://updated.test")
	request("/base/", 200, "literal combined frontend TWO ENV_202", "", "", "")
	check(beta, "BETA", "literal-4101:7")
	t.Logf("ALPHA\n%s\nBETA\n%s", alpha.log.String(), beta.log.String())
}

func sessionConfig(base, appDir, trusted string) string {
	return fmt.Sprintf(`import {sveltekit} from '@sveltejs/kit/vite'; import {defineConfig} from 'vite-plus'; import skgo from '@skgo/sveltekit-adapter'; export default defineConfig({plugins:[sveltekit({adapter:skgo(),paths:{origin:process.env.ORIGIN,base:%q},appDir:%q,csrf:{trustedOrigins:[%q]},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}})]});`, base, appDir, trusted)
}

func sessionRoute(value string, initial int) string {
	return fmt.Sprintf(`package orders
import("fmt";"net/http";"strconv";"github.com/tylergannon/skgo")
var initial=%d
func get(w http.ResponseWriter,r *http.Request){if value:=r.URL.Query().Get("initial");value!=""{initial,_=strconv.Atoi(value)};fmt.Fprintf(w,"literal-%s:%%d",initial)}
var _=skgo.GET(get)
`, initial, value)
}

type sessionLog struct {
	sync.Mutex
	b strings.Builder
}

func (l *sessionLog) Write(p []byte) (int, error) { l.Lock(); defer l.Unlock(); return l.b.Write(p) }
func (l *sessionLog) String() string              { l.Lock(); defer l.Unlock(); return l.b.String() }
func (l *sessionLog) lastPID() int {
	matches := regexp.MustCompile(`(?:Go pid |recovered Go pid )(\d+)`).FindAllStringSubmatch(l.String(), -1)
	if len(matches) == 0 {
		return 0
	}
	p, _ := strconv.Atoi(matches[len(matches)-1][1])
	return p
}
func (l *sessionLog) vitePID() int {
	matches := regexp.MustCompile(`Vite child pid (\d+)`).FindAllStringSubmatch(l.String(), -1)
	if len(matches) == 0 {
		return 0
	}
	p, _ := strconv.Atoi(matches[len(matches)-1][1])
	return p
}
