//go:build skgo_dev_integration

package devserver

import (
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
		if e = os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(app, "web/node_modules")); e != nil {
			t.Fatal(e)
		}
		write("web/vite.config.ts", `import {sveltekit} from '@sveltejs/kit/vite'; import {defineConfig} from 'vite-plus'; import skgo from '@skgo/sveltekit-adapter'; export default defineConfig({plugins:[sveltekit({adapter:skgo(),paths:{origin:process.env.ORIGIN},experimental:{remoteFunctions:true},compilerOptions:{experimental:{async:true}}})]});`)
		write("web/src/app.html", `<!doctype html><html><head>%sveltekit.head%</head><body><div>%sveltekit.body%</div></body></html>`)
		write("web/src/routes/+page.svelte", `<h1>literal session frontend</h1>`)
		write("locals/locals.go", "package locals\ntype Locals struct{}\n")
		write("generated/config.go", "package generated\n//go:generate go tool skgo generate --web ../web --locals-package example.com/sessionfixture/locals --locals-type Locals\n")
		write("web/src/routes/orders/server.go", sessionRoute("4101", 7))
		write("server.go", fmt.Sprintf(`package sessionfixture
import("io/fs";"net/http";"github.com/tylergannon/skgo"; generated "example.com/sessionfixture/generated")
func NewHandler(dist fs.FS,origin string)(http.Handler,error){
 manifest,err:=skgo.ReadManifest(dist);if err!=nil{return nil,err}
 remotes,err:=skgo.NewRemotes(manifest.RemoteConfig(origin),generated.Remotes()...);if err!=nil{return nil,err}
 loads,err:=skgo.NewLoads(manifest.LoadConfig(origin),generated.Loads()...);if err!=nil{return nil,err}
 actions,err:=skgo.NewActions(generated.Actions()...);if err!=nil{return nil,err}
 renderer,err:=skgo.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{Actions:actions});if err!=nil{return nil,err}
 pages,err:=skgo.NewStaticHandler(dist,skgo.WithSSR(renderer));if err!=nil{return nil,err}
 endpoints,err:=skgo.NewEndpoints(manifest.EndpointConfig(origin),generated.Endpoints()...);if err!=nil{return nil,err}
 cfg:=manifest.HandleConfig();cfg.Origin=origin;cfg.Loads=loads
 app:=generated.RequestBoundary(cfg,loads.Intercept(remotes.Intercept(endpoints.Intercept(pages))))
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Session",%q);w.Header().Set("X-Origin",origin);app.ServeHTTP(w,r)}),nil
}
`, label))
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
		for time.Now().Before(deadline) {
			r, e := http.Get(s.origin + "/orders")
			if e == nil {
				b, _ := io.ReadAll(r.Body)
				r.Body.Close()
				if r.StatusCode == 200 && string(b) == body && r.Header.Get("X-Session") == label && r.Header.Get("X-Origin") == s.origin {
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
	if e := os.WriteFile(filepath.Join(alpha.app, "web/src/routes/+page.svelte"), []byte(`<h1>literal combined frontend TWO</h1>`), 0644); e != nil {
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

	check(alpha, "ALPHA", "literal-7407:9")
	check(beta, "BETA", "literal-4101:7")
	t.Logf("ALPHA\n%s\nBETA\n%s", alpha.log.String(), beta.log.String())
}

func sessionRoute(value string, initial int) string {
	return fmt.Sprintf(`package orders
import("fmt";"net/http";"github.com/tylergannon/skgo")
var initial=%d
func get(w http.ResponseWriter,r *http.Request){fmt.Fprintf(w,"literal-%s:%%d",initial)}
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
