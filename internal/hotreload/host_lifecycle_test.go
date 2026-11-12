//go:build skgo_qualification

package hotreload

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/gen"
)

// Exercise handler reconstruction through the generated executable's control
// pipe, rather than Install (which replaces code without rebuilding handlers).
func TestHostReloadRetiresIdleApplicationResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(work, "app")
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mod, err := os.ReadFile(filepath.Join(root, "example/go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.mod", strings.ReplaceAll(strings.ReplaceAll(string(mod), "module github.com/tylergannon/skgo/example", "module example.com/lifecycle"), "=> ../", "=> "+filepath.ToSlash(root)))
	sums, err := os.ReadFile(filepath.Join(root, "example/go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(sums))
	t.Setenv("GOWORK", "off")
	t.Setenv("SKGO_LIFECYCLE_RESOURCES", work)
	write("web/package.json", `{"name":"host-lifecycle-contract","type":"module"}`)
	write("locals/locals.go", "package locals\ntype Locals struct{}\n")
	write("web/src/routes/probe/server.go", `package probe
import("net/http";"github.com/tylergannon/skgo")
func get(w http.ResponseWriter,r *http.Request){w.Write([]byte("probe"))}
var _=skgo.GET(get)
`)
	write("server.go", lifecycleResourceApplication)
	write("first/generation", "handler-101")
	write("second/generation", "handler-202")
	project, err := gen.OpenProject(gen.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/lifecycle/locals", LocalsType: "Locals", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer project.Close()
	build, err := project.Prepare(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer build.Cancel()
	defer build.Inputs.Close()
	host, err := CompileHost(ctx, HostOptions{Inputs: build.Inputs, WorkDir: filepath.Join(work, "host"), Origin: "http://localhost", NativeKit: true, Output: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if err := host.Start(ctx, filepath.Join(app, "first")); err != nil {
		t.Fatal(err)
	}
	pid := strconv.Itoa(host.PID)
	client := &http.Client{Timeout: 10 * time.Second}
	request := func(path, want string) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || string(body) != want || response.Header.Get("X-App-Pid") != pid {
			t.Fatalf("GET %s: status=%d body=%q pid=%q err=%v; want status=200 body=%q pid=%s", path, response.StatusCode, body, response.Header.Get("X-App-Pid"), err, want, pid)
		}
		t.Logf("GET %s: status=200 body=%q child=%s", path, body, pid)
	}
	// Opening and locking a distinct file description observes the actual OS
	// resource independently of the application's Close receipt.
	resourceOpen := func(generation string) bool {
		t.Helper()
		file, err := os.OpenFile(filepath.Join(work, generation+".lock"), os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true
		}
		if err != nil {
			t.Fatal(err)
		}
		return false // file.Close releases this observer's lock.
	}
	request("/", "handler-101")
	request("/state", "completed=1 active=0")
	if !resourceOpen("handler-101") {
		t.Fatal("initial application's resource was not held")
	}
	if err := host.Reload(ctx, filepath.Join(app, "second")); err != nil {
		t.Fatal(err)
	}
	t.Log("Host.Reload acknowledged success")
	request("/", "handler-202")
	request("/state", "completed=1 active=0")
	oldOpen, activeOpen := resourceOpen("handler-101"), resourceOpen("handler-202")
	closed, err := os.ReadFile(filepath.Join(work, "closed"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	t.Logf("resources after reload: predecessor_open=%t replacement_open=%t close_receipts=%q", oldOpen, activeOpen, closed)
	if oldOpen || !activeOpen || string(closed) != "handler-101\n" {
		t.Fatalf("idle application retirement: predecessor_open=%t replacement_open=%t close_receipts=%q; want false, true, %q", oldOpen, activeOpen, closed, "handler-101\n")
	}
}

const lifecycleResourceApplication = `package lifecycle
import("fmt";"io";"io/fs";"net/http";"os";"path/filepath";"strconv";"sync/atomic";"syscall";"github.com/tylergannon/skgo";"example.com/lifecycle/generated")
// Keep a raw descriptor so an os.File finalizer cannot hide missing Close calls.
type resourceHandler struct{generation string;resource int;active,completed atomic.Int64}
var _ io.Closer=(*resourceHandler)(nil)
func(h *resourceHandler)ServeHTTP(w http.ResponseWriter,r *http.Request){
 w.Header().Set("X-App-Pid",strconv.Itoa(os.Getpid()))
 if r.URL.Path=="/state"{fmt.Fprintf(w,"completed=%d active=%d",h.completed.Load(),h.active.Load());return}
 h.active.Add(1);defer func(){h.completed.Add(1);h.active.Add(-1)}()
 fmt.Fprint(w,h.generation)
}
func(h *resourceHandler)Close()error{
 if err:=syscall.Close(h.resource);err!=nil{return err}
 file,err:=os.OpenFile(filepath.Join(os.Getenv("SKGO_LIFECYCLE_RESOURCES"),"closed"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{return err};defer file.Close()
 _,err=fmt.Fprintln(file,h.generation);return err
}
func NewDevelopmentHandler(dist fs.FS,origin string,native *skgo.NativeDevelopment)(http.Handler,error){
 generation,err:=fs.ReadFile(dist,"generation");if err!=nil{return nil,err}
 manifest:=skgo.Manifest{Remotes:[]string{}}
 loads,err:=skgo.NewLoads(manifest.LoadConfig(origin),generated.Loads()...);if err!=nil{return nil,err}
 remotes,err:=skgo.NewRemotes(manifest.RemoteConfig(origin),generated.Remotes()...);if err!=nil{return nil,err}
 if _,err=native.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{});err!=nil{return nil,err}
 descriptor,err:=syscall.Open(filepath.Join(os.Getenv("SKGO_LIFECYCLE_RESOURCES"),string(generation)+".lock"),syscall.O_CREAT|syscall.O_RDWR,0600);if err!=nil{return nil,err}
 if err=syscall.Flock(descriptor,syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{syscall.Close(descriptor);return nil,err}
 return &resourceHandler{generation:string(generation),resource:descriptor},nil
}
`
