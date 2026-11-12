//go:build skgo_qualification

package hotreload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	// Parent-owned gates make HTTP and native logical work deterministic. They
	// hold real child work, not a timer or an assumed request duration.
	httpEntered, httpRelease := make(chan struct{}), make(chan struct{})
	bridgeEntered, bridgeRelease := make(chan struct{}), make(chan struct{})
	shutdownEntered, shutdownRelease := make(chan struct{}), make(chan struct{})
	cancelEntered, cancelRelease := make(chan struct{}), make(chan struct{})
	interruptEntered, interruptRelease := make(chan struct{}), make(chan struct{})
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var entered, release chan struct{}
		switch r.URL.Path {
		case "/http":
			entered, release = httpEntered, httpRelease
		case "/handler-303":
			entered, release = bridgeEntered, bridgeRelease
		case "/handler-505":
			entered, release = shutdownEntered, shutdownRelease
		case "/handler-606":
			entered, release = cancelEntered, cancelRelease
		case "/handler-707":
			entered, release = interruptEntered, interruptRelease
		default:
			http.Error(w, "unexpected gate", 500)
			return
		}
		close(entered)
		<-release
		io.WriteString(w, "literal-resource-use-713")
	}))
	defer gate.Close()
	defer func() {
		for _, ch := range []chan struct{}{httpRelease, bridgeRelease, shutdownRelease, cancelRelease, interruptRelease} {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
	}()
	t.Setenv("SKGO_LIFECYCLE_GATE", gate.URL)
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
	write("third/generation", "handler-303")
	write("rejected/generation", "handler-404")
	write("error-owner/generation", "handler-405")
	write("last/generation", "handler-505")
	write("cancelled/generation", "handler-606")
	write("interrupted/generation", "handler-707")
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
	host, err := CompileHost(ctx, HostOptions{Inputs: build.Inputs, WorkDir: filepath.Join(work, "host"), Origin: "http://localhost", NativeKit: true, ServiceSecret: "lifecycle-secret", Output: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Release parent-owned gates before joining the child on an assertion failure.
		for _, ch := range []chan struct{}{httpRelease, bridgeRelease, shutdownRelease, cancelRelease, interruptRelease} {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
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
	// Continue the retained baseline in the same generated executable.
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	receipts := func(want string) {
		t.Helper()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			raw, e := os.ReadFile(filepath.Join(work, "closed"))
			if e != nil {
				t.Fatal(e)
			}
			if string(raw) == want {
				t.Logf("close_receipts=%q", raw)
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatalf("close receipts=%q want=%q", raw, want)
			}
		}
	}
	held := make(chan string, 1)
	go func() {
		response, e := client.Get(host.URL + "/hold")
		if e != nil {
			held <- e.Error()
			return
		}
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		if e != nil {
			held <- e.Error()
			return
		}
		held <- string(body)
	}()
	wait(httpEntered)
	if err := host.Reload(ctx, filepath.Join(app, "third")); err != nil {
		t.Fatal(err)
	}
	request("/", "handler-303")
	if !resourceOpen("handler-202") || !resourceOpen("handler-303") {
		t.Fatal("held request or replacement lost its resource")
	}
	t.Log("replacement handler-303 serves while handler-202 is deterministically held; both locks held")
	close(httpRelease)
	select {
	case got := <-held:
		if got != "held-handler-202:literal-resource-use-713" {
			t.Fatal(got)
		}
		t.Logf("held response=%q", got)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	receipts("handler-101\nhandler-202\n")
	if resourceOpen("handler-202") {
		t.Fatal("held predecessor did not settle")
	}
	for _, rejected := range []struct{ dir, diagnostic string }{
		{"rejected", "native development requires the application's SSR registries"},
		{"error-owner", "literal-constructor-rejection-405"},
	} {
		err := host.Reload(ctx, filepath.Join(app, rejected.dir))
		if err == nil || !strings.Contains(err.Error(), rejected.diagnostic) {
			t.Fatalf("rejection: %v", err)
		}
		t.Logf("candidate rejected: %v", err)
		request("/", "handler-303")
	}
	receipts("handler-101\nhandler-202\nhandler-404\nhandler-405\n")
	if resourceOpen("handler-404") || resourceOpen("handler-405") || !resourceOpen("handler-303") {
		t.Fatal("rejection ownership")
	}
	begin := func() {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, "POST", host.URL+"/_skgo/dev/begin", strings.NewReader(`{"url":"http://localhost/bridge","method":"GET"}`))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer lifecycle-secret")
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var answer struct {
			Handle  string
			Resolve bool
		}
		if e = json.NewDecoder(response.Body).Decode(&answer); e != nil || response.StatusCode != 200 || !answer.Resolve || answer.Handle == "" {
			t.Fatalf("begin: %+v status=%d err=%v", answer, response.StatusCode, e)
		}
		t.Logf("native /begin HTTP returned 200 resolve=true; logical middleware still owns resource; handle=%s", answer.Handle)
	}
	begin()
	reloaded := make(chan error, 1)
	go func() { reloaded <- host.Reload(ctx, filepath.Join(app, "last")) }()
	wait(bridgeEntered) // Retirement cancelled the logical request; its defer is still using the owner.
	request("/", "handler-505")
	if !resourceOpen("handler-303") || !resourceOpen("handler-505") {
		t.Fatal("bridge owner closed before its last use")
	}
	select {
	case err := <-reloaded:
		t.Fatalf("reload returned before native bridge joined: %v", err)
	default:
	}
	t.Log("after outer /begin returned, actual native downstream teardown remains held; handler-303 lock held while handler-505 serves")
	close(bridgeRelease)
	if err := <-reloaded; err != nil {
		t.Fatal(err)
	}
	receipts("handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\n")
	if resourceOpen("handler-303") {
		t.Fatal("native predecessor did not settle")
	}
	begin()
	stopped := make(chan error, 1)
	go func() { stopped <- host.Close() }()
	wait(shutdownEntered)
	if !resourceOpen("handler-505") {
		t.Fatal("shutdown freed active native resource")
	}
	select {
	case err := <-stopped:
		t.Fatalf("Host.Close returned before native teardown: %v", err)
	default:
	}
	close(shutdownRelease)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	} // idempotent parent close
	receipts("handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\n")
	if resourceOpen("handler-505") {
		t.Fatal("last resource remains locked after Host.Close")
	}
	events, err := os.ReadFile(filepath.Join(work, "events"))
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := "closed-handler-101\nused-handler-202\nclosed-handler-202\nclosed-handler-404\nclosed-handler-405\nused-handler-303\nclosed-handler-303\nused-handler-505\nclosed-handler-505\n"
	if string(events) != wantEvents {
		t.Fatalf("resource event ordering=%q want=%q", events, wantEvents)
	}
	t.Logf("normal parent Host.Close reaped child=%s; final lock free; events=%q", pid, events)

	// Reuse the compiled executable for a second session. Normal CLI shutdown
	// cancels this context before calling Close; it must take the same drain path.
	host = &Host{options: host.options, binary: host.binary}
	childCtx, cancelChild := context.WithCancel(ctx)
	defer cancelChild()
	if err := host.Start(childCtx, filepath.Join(app, "cancelled")); err != nil {
		t.Fatal(err)
	}
	pid = strconv.Itoa(host.PID)
	request("/", "handler-606")
	begin()
	cancelChild()
	wait(cancelEntered)
	if !resourceOpen("handler-606") {
		t.Fatal("context cancellation killed the native owner before its last use")
	}
	select {
	case <-host.Done():
		t.Fatal("cancelled child exited before native work finished")
	default:
	}
	t.Log("session context cancelled; actual native teardown reached its gate with handler-606 lock still held")
	closedAfterCancel := make(chan error, 1)
	go func() { closedAfterCancel <- host.Close() }()
	close(cancelRelease)
	if err := <-closedAfterCancel; err != nil {
		t.Fatal(err)
	}
	receipts("handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\nhandler-606\n")
	if resourceOpen("handler-606") {
		t.Fatal("cancelled session resource remains locked")
	}
	events, err = os.ReadFile(filepath.Join(work, "events"))
	if err != nil {
		t.Fatal(err)
	}
	if string(events) != wantEvents+"used-handler-606\nclosed-handler-606\n" {
		t.Fatalf("cancelled resource event ordering=%q", events)
	}
	t.Logf("session cancellation and Host.Close reaped child=%s; handler-606 lock free; last resource use precedes exactly one Close", pid)

	// Terminal Ctrl-C also reaches the child's inherited process group. Send
	// the real signal only to our owned child so the test runner stays alive.
	host = &Host{options: host.options, binary: host.binary}
	if err := host.Start(ctx, filepath.Join(app, "interrupted")); err != nil {
		t.Fatal(err)
	}
	pid = strconv.Itoa(host.PID)
	request("/", "handler-707")
	begin()
	parentGroup, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	childGroup, err := syscall.Getpgid(host.PID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("terminal delivery: parent_pgid=%d child_pgid=%d; sending actual SIGINT to owned child=%s", parentGroup, childGroup, pid)
	if err := syscall.Kill(host.PID, syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	wait(interruptEntered)
	if !resourceOpen("handler-707") {
		t.Fatal("SIGINT freed native owner before its last use")
	}
	select {
	case <-host.Done():
		t.Fatal("SIGINT killed child before native teardown")
	default:
	}
	close(interruptRelease)
	wait(host.Done())
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	receipts("handler-101\nhandler-202\nhandler-404\nhandler-405\nhandler-303\nhandler-505\nhandler-606\nhandler-707\n")
	if resourceOpen("handler-707") {
		t.Fatal("interrupted resource remains locked")
	}
	events, err = os.ReadFile(filepath.Join(work, "events"))
	if err != nil {
		t.Fatal(err)
	}
	if string(events) != wantEvents+"used-handler-606\nclosed-handler-606\nused-handler-707\nclosed-handler-707\n" {
		t.Fatalf("interrupted resource event ordering=%q", events)
	}
	t.Logf("actual SIGINT gracefully reaped child=%s; handler-707 lock free; last resource use precedes exactly one Close", pid)

}

const lifecycleResourceApplication = `package lifecycle
import("fmt";"io";"io/fs";"net/http";"os";"path/filepath";"strconv";"sync/atomic";"syscall";"github.com/tylergannon/skgo";"example.com/lifecycle/generated")
// Keep a raw descriptor so an os.File finalizer cannot hide missing Close calls.
type resourceHandler struct{generation string;resource int;pages http.Handler;active,completed atomic.Int64}
var _ io.Closer=(*resourceHandler)(nil)
func(h *resourceHandler)ServeHTTP(w http.ResponseWriter,r *http.Request){
 w.Header().Set("X-App-Pid",strconv.Itoa(os.Getpid()))
 if r.URL.Path=="/state"{fmt.Fprintf(w,"completed=%d active=%d",h.completed.Load(),h.active.Load());return}
 h.active.Add(1);defer func(){h.completed.Add(1);h.active.Add(-1)}()
 if r.URL.Path=="/hold"{value:=h.use("/http");fmt.Fprint(w,"held-"+h.generation+":"+value);return}
 if r.URL.Path=="/bridge"{defer h.use("/"+h.generation);h.pages.ServeHTTP(w,r);return}
 fmt.Fprint(w,h.generation)
}
func(h *resourceHandler)event(value string){f,err:=os.OpenFile(filepath.Join(os.Getenv("SKGO_LIFECYCLE_RESOURCES"),"events"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{panic(err)};defer f.Close();if _,err=fmt.Fprintln(f,value);err!=nil{panic(err)}}
func(h *resourceHandler)use(path string)string{
 response,err:=http.Get(os.Getenv("SKGO_LIFECYCLE_GATE")+path);if err!=nil{panic(err)};defer response.Body.Close()
 body,err:=io.ReadAll(response.Body);if err!=nil{panic(err)}
 if _,err=syscall.Write(h.resource,[]byte("last-resource-use\n"));err!=nil{panic(err)}
 h.event("used-"+h.generation);return string(body)
}
func(h *resourceHandler)Close()error{
 if err:=syscall.Close(h.resource);err!=nil{return err}
 h.event("closed-"+h.generation)
 file,err:=os.OpenFile(filepath.Join(os.Getenv("SKGO_LIFECYCLE_RESOURCES"),"closed"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{return err};defer file.Close()
 _,err=fmt.Fprintln(file,h.generation);return err
}
func NewDevelopmentHandler(dist fs.FS,origin string,native *skgo.NativeDevelopment)(http.Handler,error){
 generation,err:=fs.ReadFile(dist,"generation");if err!=nil{return nil,err}
 manifest:=skgo.Manifest{Remotes:[]string{}}
 loads,err:=skgo.NewLoads(manifest.LoadConfig(origin),generated.Loads()...);if err!=nil{return nil,err}
 remotes,err:=skgo.NewRemotes(manifest.RemoteConfig(origin),generated.Remotes()...);if err!=nil{return nil,err}
 if string(generation)!="handler-404"{if _,err=native.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{});err!=nil{return nil,err}}
 pages,err:=native.NewStaticHandler(dist);if err!=nil{return nil,err}
 descriptor,err:=syscall.Open(filepath.Join(os.Getenv("SKGO_LIFECYCLE_RESOURCES"),string(generation)+".lock"),syscall.O_CREAT|syscall.O_RDWR,0600);if err!=nil{return nil,err}
 if err=syscall.Flock(descriptor,syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{syscall.Close(descriptor);return nil,err}
 h:=&resourceHandler{generation:string(generation),resource:descriptor,pages:pages}
 if string(generation)=="handler-405"{return h,fmt.Errorf("literal-constructor-rejection-405")}
 return h,nil
}
`
