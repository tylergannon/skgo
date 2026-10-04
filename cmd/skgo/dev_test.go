package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

// The application below is an ordinary skgo application written as literal
// source: a Go endpoint and a Go page action beside their routes, the
// generated bindings package, and a main that mounts what the generator
// registered. Every answer the test expects is a literal the test writes into
// a file after the session began, so no binary built earlier can contain it.

const devAppMain = `package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tylergannon/skgo"
	generated "toy/internal/skgo"
)

// routes stands in for Kit's route table: one entry for every directory under
// web/src/routes that holds the +server.ts the generator wrote.
func routes() []skgo.ManifestRoute {
	var out []skgo.ManifestRoute
	root := filepath.Join("web", "src", "routes")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "+server.ts" {
			return nil
		}
		dir, _ := filepath.Rel(root, filepath.Dir(path))
		id := "/" + filepath.ToSlash(dir)
		pattern := "^" + regexp.QuoteMeta(id) + "/?$"
		out = append(out, skgo.ManifestRoute{ID: id, Pattern: strings.ReplaceAll(pattern, "/", "\\/"), Endpoint: &skgo.ManifestEndpoint{Methods: []string{"GET"}}})
		return nil
	})
	return out
}

func main() {
	listen := flag.String("listen", "", "")
	flag.String("proxy", "", "")
	flag.String("origin", "", "")
	flag.Parse()
	endpoints, err := skgo.NewEndpoints(skgo.EndpointConfig{Dev: true, Routes: routes()}, generated.Endpoints()...)
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/__registry", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "pid=%d actions=%d endpoints=%d", os.Getpid(), len(generated.Actions()), len(generated.Endpoints()))
	})
	mux.Handle("/", endpoints.Intercept(http.NotFoundHandler()))
	log.Fatal(http.ListenAndServe(*listen, mux))
}
`

const devEndpointOne = `package thing

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Go-Revision", "one")
	_, _ = w.Write([]byte("Go endpoint revision one"))
}

var _ = skgo.GET(get)
`

const devEndpointTwo = `package thing

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Go-Revision", "two")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("Go endpoint revision two"))
}

var _ = skgo.GET(get)
`

const devNewEndpoint = `package godevendpoint

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func get(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("New Go endpoint revision one"))
}

var _ = skgo.GET(get)
`

const devActionOne = `package form

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Result struct {
	Message string ` + "`json:\"message\"`" + `
}

func revise(context.Context) (Result, error) { return Result{Message: "Go action revision one"}, nil }

var _ = skgo.Action(revise)
`

const devActionTwo = `package form

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Result struct {
	Message  string ` + "`json:\"message\"`" + `
	Revision int    ` + "`json:\"revision\"`" + `
}

func revise(context.Context) (Result, error) {
	return Result{Message: "Go action revision two", Revision: 2}, nil
}

func added(context.Context) (Result, error) { return Result{Message: "Added Go action"}, nil }

var _ = skgo.Action(revise)
var _ = skgo.Action(added)
`

const devFakeVite = `package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	if os.Getenv("SKGO_FAKE_VITE_MODE") == "stay" {
		fmt.Fprintln(os.Stdout, "FAKE_VITE_READY")
		for {
			time.Sleep(time.Hour)
		}
	}
	fmt.Fprintln(os.Stderr, "FAKE_VITE_FAILURE_CAUSE")
	time.Sleep(250 * time.Millisecond)
	code, _ := strconv.Atoi(os.Getenv("SKGO_FAKE_VITE_EXIT"))
	os.Exit(code)
}
`

type devApp struct {
	t    *testing.T
	root string
	env  []string
}

func newDevApp(t *testing.T) *devApp {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(filepath.Join(repo, "example", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(repo, "example", "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	gomod := regexp.MustCompile(`(?m)^module .*$`).ReplaceAllString(string(example), "module toy")
	const relative = "replace github.com/tylergannon/skgo => ../"
	if !strings.Contains(gomod, relative) {
		t.Fatalf("example/go.mod no longer contains %q; update this test", relative)
	}
	gomod = strings.Replace(gomod, relative, "replace github.com/tylergannon/skgo => "+repo, 1)

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &devApp{t: t, root: root, env: append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")}
	app.write("go.mod", gomod)
	app.write("go.sum", string(sum))
	app.write("cmd/main.go", devAppMain)
	app.write("internal/skgo/config.go", "package skgo\n")
	app.write("web/src/routes/api/thing/server.go", devEndpointOne)
	app.write("web/src/routes/form/page.server.go", devActionOne)
	return app
}

func (a *devApp) write(rel, content string) {
	a.t.Helper()
	path := filepath.Join(a.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		a.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		a.t.Fatal(err)
	}
}

func (a *devApp) read(rel string) string {
	a.t.Helper()
	raw, err := os.ReadFile(filepath.Join(a.root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	return string(raw)
}

type devReply struct {
	status int
	body   string
	header http.Header
}

func getReply(url string) devReply {
	resp, err := http.Get(url)
	if err != nil {
		return devReply{status: -1, body: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return devReply{status: resp.StatusCode, body: string(body), header: resp.Header}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (a *devApp) until(url string, accept func(devReply) bool, what string, log *lockedBuffer) devReply {
	a.t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	var last devReply
	for time.Now().Before(deadline) {
		last = getReply(url)
		if accept(last) {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	a.t.Fatalf("%s: last answer %d %q %v\nskgo dev log:\n%s", what, last.status, last.body, last.header, log.String())
	return last
}

func freeHostPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func buildDevFakeVite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte(devFakeVite), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "fake-vite")
	build := exec.Command("go", "build", "-o", bin, source)
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake Vite child: %v\n%s", err, output)
	}
	return bin
}

func launchDevWithVite(t *testing.T, app *devApp, vite, mode, exit string) (*exec.Cmd, *lockedBuffer, <-chan struct{}, *error) {
	t.Helper()
	addr := freeHostPort(t)
	base := "http://" + addr
	log := &lockedBuffer{}
	dev := exec.Command(skgoBin, "dev", "--root", app.root, "--web", "web", "--cmd", "./cmd",
		"--listen", addr, "--origin", base, "--vite", vite, "--vite-port", "0")
	dev.Env = append(app.env, "SKGO_FAKE_VITE_MODE="+mode, "SKGO_FAKE_VITE_EXIT="+exit)
	dev.Stdout, dev.Stderr = log, log
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = dev.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		_ = dev.Process.Signal(syscall.SIGINT)
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			_ = dev.Process.Kill()
			<-done
		}
	})
	return dev, log, done, &waitErr
}

func awaitDevLog(t *testing.T, done <-chan struct{}, log *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(log.String(), want) {
			return
		}
		select {
		case <-done:
			t.Fatalf("skgo dev exited before logging %q:\n%s", want, log.String())
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in skgo dev output:\n%s", want, log.String())
}

// One `skgo dev` launch, the command the scaffold's recipe runs, over an app
// whose Go endpoint and action are edited, added and removed while it runs. A
// built binary of the same source runs beside it and must stay at what it was
// built with.
func TestSkgoDevAdoptsAuthoredEndpointsAndActionsInOneLaunch(t *testing.T) {
	app := newDevApp(t)

	// The built binary: what a deployment of the starting source answers, for
	// as long as the developer edits the source.
	gen := exec.Command(skgoBin, "generate", "--web", filepath.Join(app.root, "web"), "--out", ".", "--quiet")
	gen.Dir = filepath.Join(app.root, "internal", "skgo")
	gen.Env = app.env
	if output, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("skgo generate: %v\n%s", err, output)
	}
	built := filepath.Join(t.TempDir(), "built")
	build := exec.Command("go", "build", "-o", built, "./cmd")
	build.Dir = app.root
	build.Env = app.env
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	builtAddr := freeHostPort(t)
	prebuilt := exec.Command(built, "-listen", builtAddr)
	prebuilt.Dir = app.root
	if err := prebuilt.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prebuilt.Process.Kill(); _ = prebuilt.Wait() })

	vite := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(vite.Close)
	addr := freeHostPort(t)
	base := "http://" + addr
	log := &lockedBuffer{}
	dev := exec.Command(skgoBin, "dev", "--root", app.root, "--web", "web", "--cmd", "./cmd",
		"--listen", addr, "--origin", base, "--vite-url", vite.URL)
	dev.Env = app.env
	dev.Stdout, dev.Stderr = log, log
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- dev.Wait() }()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = dev.Process.Signal(syscall.SIGINT)
			select {
			case <-exited:
			case <-time.After(20 * time.Second):
				_ = dev.Process.Kill()
			}
		}
	})

	registry := func(want string) func(devReply) bool {
		return func(r devReply) bool { return r.status == 200 && strings.HasSuffix(r.body, want) }
	}
	endpoint := func(status int, body, revision string) func(devReply) bool {
		return func(r devReply) bool {
			return r.status == status && r.body == body && r.header.Get("X-Go-Revision") == revision
		}
	}
	builtSays := func() devReply { return getReply("http://" + builtAddr + "/api/thing") }
	app.until("http://"+builtAddr+"/api/thing", endpoint(200, "Go endpoint revision one", "one"), "the built binary", log)

	app.until(base+"/api/thing", endpoint(200, "Go endpoint revision one", "one"), "first build", log)
	app.until(base+"/__registry", registry("actions=1 endpoints=1"), "first registry", log)

	// An endpoint's status, body and header change.
	app.write("web/src/routes/api/thing/server.go", devEndpointTwo)
	app.until(base+"/api/thing", endpoint(202, "Go endpoint revision two", "two"), "edited endpoint", log)

	// An action's result type changes and a second action appears: the
	// declarations Kit reads and the registry the server mounts both follow.
	if declaration := app.read("web/src/routes/form/+page.server.ts"); strings.Contains(declaration, "revision") || strings.Contains(declaration, "added") {
		t.Fatalf("the starting action declaration already carries the later signature:\n%s", declaration)
	}
	app.write("web/src/routes/form/page.server.go", devActionTwo)
	app.until(base+"/__registry", registry("actions=2 endpoints=1"), "added action registered", log)
	declaration := app.read("web/src/routes/form/+page.server.ts")
	for _, want := range []string{"revision: number", "added:"} {
		if !strings.Contains(declaration, want) {
			t.Fatalf("the generated action declaration lacks %q after the signature changed:\n%s", want, declaration)
		}
	}

	// A route that was absent when the session began, with its Go handler.
	if r := getReply(base + "/go-dev-endpoint"); r.status != http.StatusNotFound {
		t.Fatalf("/go-dev-endpoint answered %d %q before it existed", r.status, r.body)
	}
	app.write("web/src/routes/go-dev-endpoint/server.go", devNewEndpoint)
	app.until(base+"/go-dev-endpoint", endpoint(200, "New Go endpoint revision one", ""), "added endpoint", log)
	app.until(base+"/__registry", registry("actions=2 endpoints=2"), "added endpoint registered", log)
	if !strings.Contains(app.read("web/src/routes/go-dev-endpoint/+server.ts"), "export const GET") {
		t.Fatal("the generator wrote no +server.ts for the added endpoint")
	}
	if err := os.RemoveAll(filepath.Join(app.root, "web", "src", "routes", "go-dev-endpoint")); err != nil {
		t.Fatal(err)
	}
	app.until(base+"/go-dev-endpoint", func(r devReply) bool { return r.status == http.StatusNotFound }, "removed endpoint", log)
	app.until(base+"/__registry", registry("actions=2 endpoints=1"), "removed endpoint unregistered", log)

	// A compile error is reported with its file, and the same launch recovers.
	good := app.read("web/src/routes/api/thing/server.go")
	app.write("web/src/routes/api/thing/server.go", strings.Replace(good, `w.WriteHeader(http.StatusAccepted)`, `w.WriteHeader("accepted")`, 1))
	failed := app.until(base+"/api/thing", func(r devReply) bool { return r.status == http.StatusInternalServerError }, "compile error", log)
	if failed.header.Get("X-Skgo-Dev-Error") == "" || !strings.Contains(failed.body, "server.go") || !strings.Contains(failed.body, "accepted") {
		t.Fatalf("the compile error is not actionable: %v\n%s", failed.header, failed.body)
	}
	app.write("web/src/routes/api/thing/server.go", strings.Replace(good, "revision two", "revision three", 1))
	app.until(base+"/api/thing", endpoint(202, "Go endpoint revision three", "two"), "recovery", log)

	// The built binary never moved.
	if r := builtSays(); !endpoint(200, "Go endpoint revision one", "one")(r) {
		t.Fatalf("the built binary changed with the source: %d %q %v", r.status, r.body, r.header)
	}

	// Every application the session started is gone once it stops.
	var pids []int
	for _, match := range regexp.MustCompile(`started application pid (\d+)`).FindAllStringSubmatch(log.String(), -1) {
		pid, _ := strconv.Atoi(match[1])
		pids = append(pids, pid)
	}
	if len(pids) < 6 {
		t.Fatalf("the session started %d applications for six builds:\n%s", len(pids), log.String())
	}
	if err := dev.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(30 * time.Second):
		t.Fatalf("skgo dev did not stop:\n%s", log.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for _, pid := range pids {
		for processAlive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("application pid %d outlived the session", pid)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if _, err := http.Get(base + "/api/thing"); err == nil {
		t.Fatal("the public address still answers after shutdown")
	}
	if r := builtSays(); r.status != 200 {
		t.Fatalf("stopping the session stopped the built binary: %d %q", r.status, r.body)
	}
}

func TestSkgoDevUnexpectedViteExitReturnsFailure(t *testing.T) {
	for _, exitCode := range []string{"0", "7"} {
		t.Run("exit-"+exitCode, func(t *testing.T) {
			app := newDevApp(t)
			vite := buildDevFakeVite(t)
			_, log, done, waitErr := launchDevWithVite(t, app, vite, "exit", exitCode)
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatalf("skgo dev did not stop after Vite exited:\n%s", log.String())
			}
			if *waitErr == nil {
				t.Fatalf("skgo dev succeeded after its Vite child exited unexpectedly:\n%s", log.String())
			}
			var exitErr *exec.ExitError
			if !errors.As(*waitErr, &exitErr) || exitErr.ExitCode() == 0 {
				t.Fatalf("skgo dev returned %v, want nonzero process exit:\n%s", *waitErr, log.String())
			}
			if !strings.Contains(log.String(), "FAKE_VITE_FAILURE_CAUSE") {
				t.Fatalf("the child failure cause was not preserved in output:\n%s", log.String())
			}
			if !strings.Contains(log.String(), "skgo dev: vite exited unexpectedly:") {
				t.Fatalf("the supervisor did not report the unexpected Vite exit:\n%s", log.String())
			}
			if exitCode == "0" && !strings.Contains(log.String(), "process exited successfully") {
				t.Fatalf("a clean but unexpected Vite exit had no cause in the supervisor log:\n%s", log.String())
			}
			if exitCode != "0" && !strings.Contains(log.String(), "exit status "+exitCode) {
				t.Fatalf("the Vite exit status was lost:\n%s", log.String())
			}
		})
	}
}

func TestSkgoDevExternalCancellationRemainsCleanWithViteChild(t *testing.T) {
	app := newDevApp(t)
	vite := buildDevFakeVite(t)
	dev, log, done, waitErr := launchDevWithVite(t, app, vite, "stay", "0")
	awaitDevLog(t, done, log, "FAKE_VITE_READY")
	awaitDevLog(t, done, log, "skgo dev: serving http://")
	if err := dev.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("send clean cancellation: %v (exit %v):\n%s", err, *waitErr, log.String())
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("skgo dev did not stop after external cancellation:\n%s", log.String())
	}
	if *waitErr != nil {
		t.Fatalf("external cancellation returned %v, want clean exit:\n%s", *waitErr, log.String())
	}
	if strings.Contains(log.String(), "vite exited unexpectedly:") {
		t.Fatalf("intentional Vite shutdown was reported as unexpected:\n%s", log.String())
	}
}
