package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type instance struct {
	label, origin, dir, web, goSock, viteSock string
	goServer                                  *http.Server
	public                                    *http.Server
	node                                      *exec.Cmd
	done                                      chan error
	mu                                        sync.Mutex
}

var owned string

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func write(path, body string) {
	must(os.MkdirAll(filepath.Dir(path), 0755))
	must(os.WriteFile(path, []byte(body), 0644))
}
func (a *instance) startGo(literal string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ln, e := net.Listen("unix", a.goSock)
	must(e)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cookie, _ := r.Cookie("session")
		cv := "NONE"
		if cookie != nil {
			cv = cookie.Value
		}
		w.Header().Add("Set-Cookie", "go_one=one; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "go_two=two; Path=/; SameSite=Lax")
		w.Header().Set("X-Go-Instance", a.label)
		json.NewEncoder(w).Encode(map[string]any{"literal": literal, "label": a.label, "cookie": cv, "auth": r.Header.Get("Authorization"), "marker": r.Header.Get("X-Poc-Request"), "host": r.Host, "method": r.Method, "path": r.URL.RequestURI(), "body": string(body)})
	})}
	a.goServer = server
	go server.Serve(ln)
}
func udsProxy(socket string) *httputil.ReverseProxy {
	u, _ := url.Parse("http://unix")
	return &httputil.ReverseProxy{Rewrite: func(p *httputil.ProxyRequest) { p.SetURL(u); p.Out.Host = p.In.Host }, Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "UDS upstream unavailable: "+e.Error(), 503)
	}}
}
func start(ctx context.Context, repo, session, label string, port int, wrong string) *instance {
	a := &instance{label: label, origin: fmt.Sprintf("http://127.0.0.1:%d", port), dir: filepath.Join(session, label), web: filepath.Join(owned, "runtime", label), done: make(chan error, 1)}
	must(os.MkdirAll(a.dir, 0700))
	a.goSock = filepath.Join(a.dir, "go.sock")
	a.viteSock = filepath.Join(a.dir, "vite.sock")
	must(os.MkdirAll(a.web, 0755))
	pkg, e := os.ReadFile(filepath.Join(repo, "example/web/package.json"))
	must(e)
	write(filepath.Join(a.web, "package.json"), string(pkg))
	must(os.Symlink(filepath.Join(repo, "example/web/node_modules"), filepath.Join(a.web, "node_modules")))
	server, e := os.ReadFile(filepath.Join(owned, "server.js"))
	must(e)
	write(filepath.Join(a.web, "server.js"), string(server))
	hook, e := os.ReadFile(filepath.Join(owned, "hooks.server.js"))
	must(e)
	write(filepath.Join(a.web, "src/hooks.server.js"), string(hook))
	write(filepath.Join(a.web, "vite.config.js"), `import {sveltekit} from '@sveltejs/kit/vite';import {defineConfig} from 'vite-plus';export default defineConfig({cacheDir:process.env.INSTANCE_DIR+'/cache',plugins:[sveltekit({paths:{origin:process.env.PUBLIC_ORIGIN},outDir:process.env.INSTANCE_DIR+'/kit'})]});`)
	write(filepath.Join(a.web, "src/app.html"), `<!doctype html><html><head>%sveltekit.head%</head><body><div>%sveltekit.body%</div></body></html>`)
	write(filepath.Join(a.web, "src/routes/+page.server.js"), `export async function load({fetch}){const r=await fetch('/go/value?source=kit-load',{headers:{'x-poc-request':'LOAD_7719'}});if(!r.ok)throw new Error('Go UDS '+r.status);return {reply:await r.json()}}`)
	write(filepath.Join(a.web, "src/routes/+page.svelte"), `<script>let {data}=$props();let count=$state(0);</script><h1>UDS_FRONT_ONE</h1><p id="literal">{data.reply.literal}</p><p id="identity">{data.reply.label}</p><p id="cookie">{data.reply.cookie}</p><p id="auth">{data.reply.auth}</p><p id="marker">{data.reply.marker}</p><p id="origin">{data.reply.host}</p><button onclick={()=>count++}>Count {count}</button>`)
	literal := "ALPHA_4101"
	if label == "BETA" {
		literal = "BETA_5202"
	}
	a.startGo(literal)
	goProxy, viteProxy := udsProxy(a.goSock), udsProxy(a.viteSock)
	a.public = &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__poc/replace" {
			a.mu.Lock()
			old := a.goServer
			a.mu.Unlock()
			old.Close()
			a.startGo("ALPHA_7407")
			io.WriteString(w, "replaced")
			return
		}
		if r.URL.Path == "/__poc/stop" {
			signalProcess()
			io.WriteString(w, "stopping")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/go/") {
			goProxy.ServeHTTP(w, r)
		} else {
			viteProxy.ServeHTTP(w, r)
		}
	})}
	ln, e := net.Listen("tcp", a.public.Addr)
	must(e)
	go a.public.Serve(ln)
	log, e := os.Create(filepath.Join(owned, label+"-vite.log"))
	must(e)
	a.node = exec.CommandContext(ctx, "vp", "exec", "node", "server.js")
	a.node.Dir = a.web
	a.node.Env = append(os.Environ(), "PUBLIC_ORIGIN="+a.origin, "INSTANCE_DIR="+a.dir, "GO_SOCK="+a.goSock, "WRONG_GO_SOCK="+wrong, "VITE_SOCK="+a.viteSock, "READY_FILE="+filepath.Join(a.dir, "ready.json"))
	a.node.Stdout = log
	a.node.Stderr = log
	a.node.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	must(a.node.Start())
	go func() { a.done <- a.node.Wait(); log.Close() }()
	for end := time.Now().Add(60 * time.Second); ; {
		if raw, e := os.ReadFile(filepath.Join(a.dir, "ready.json")); e == nil {
			fmt.Printf("READY %s public=%s sockets=%s %s node=%s\n", label, a.origin, a.goSock, a.viteSock, raw)
			break
		}
		select {
		case e := <-a.done:
			panic(fmt.Sprintf("%s Vite earlyexit %v", label, e))
		default:
		}
		if time.Now().After(end) {
			panic("Vite readiness timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return a
}

var stopAll = context.CancelFunc(func() {})

func signalProcess() { stopAll() }
func main() {
	repo, e := os.Getwd()
	must(e)
	owned = filepath.Join(repo, "ephemeral/experiments/20261015-vite-uds-ssr")
	must(os.RemoveAll(filepath.Join(owned, "runtime")))
	session, e := os.MkdirTemp("/tmp", "skgo-uds-")
	must(e)
	session, e = filepath.EvalSymlinks(session)
	must(e)
	defer os.RemoveAll(session)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	stopAll = cancel
	var apps []*instance
	defer func() {
		for _, a := range apps {
			a.public.Close()
			a.goServer.Close()
			syscall.Kill(-a.node.Process.Pid, syscall.SIGTERM)
			select {
			case <-a.done:
			case <-time.After(3 * time.Second):
				syscall.Kill(-a.node.Process.Pid, syscall.SIGKILL)
				<-a.done
			}
		}
	}()
	alpha := start(ctx, repo, session, "ALPHA", 57331, filepath.Join(session, "BETA/go.sock"))
	apps = append(apps, alpha)
	beta := start(ctx, repo, session, "BETA", 57332, alpha.goSock)
	apps = append(apps, beta)
	verify(alpha, beta)
	write(filepath.Join(owned, "live.json"), fmt.Sprintf(`{"session":%q,"alpha":%q,"beta":%q}`, session, alpha.origin, beta.origin))
	fmt.Println("HTTP CHECKS FINISHED; live for browser; stop via ALPHA /__poc/stop")
	<-ctx.Done()
}
func get(origin string, wrong bool) (int, string, http.Header) {
	req, _ := http.NewRequest("GET", origin+"/", nil)
	req.Header.Set("Cookie", "session=COOKIE_8831")
	req.Header.Set("Authorization", "Bearer AUTH_9942")
	if wrong {
		req.Header.Set("X-Poc-Wrong-Target", "1")
	}
	r, e := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	must(e)
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	must(e)
	return r.StatusCode, string(b), r.Header
}
func verify(a, b *instance) {
	for _, test := range []struct {
		app     *instance
		literal string
	}{{a, "ALPHA_4101"}, {b, "BETA_5202"}} {
		status, body, headers := get(test.app.origin, false)
		write(filepath.Join(owned, test.app.label+"-ssr.html"), body)
		write(filepath.Join(owned, test.app.label+"-ssr-headers.txt"), fmt.Sprint(headers))
		for _, literal := range []string{test.literal, test.app.label, "COOKIE_8831", "Bearer AUTH_9942", "LOAD_7719", strings.TrimPrefix(test.app.origin, "http://")} {
			if status != 200 || !strings.Contains(body, literal) {
				panic(fmt.Sprintf("SSR expected independent %q got status%d %s", literal, status, body))
			}
		}
		fmt.Println("PASS nativeKitSSR", test.app.label, test.literal, "cookie auth marker publichost")
	}
	status, body, _ := get(a.origin, true)
	write(filepath.Join(owned, "negative-wrong-target-ssr.html"), body)
	if status != 200 || strings.Contains(body, "ALPHA_4101") || !strings.Contains(body, "BETA_5202") {
		panic("wrongtarget control did not discriminate ALPHA literal")
	}
	fmt.Println("EXPECTED FAIL ALPHA literal on deliberately wrong Go UDS target; got BETA_5202 despite200")
	status, body, _ = get(a.origin, false)
	if status != 200 || !strings.Contains(body, "ALPHA_4101") {
		panic("restored ALPHA target failed")
	}
	fmt.Println("PASS restored ALPHA target")
}
