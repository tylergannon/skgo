package hotreload

import (
	"context"
	"errors"
	"fmt"
	"github.com/tylergannon/skgo/internal/gen"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCapturedNativeHTTPHost(t *testing.T) {
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
	t.Setenv("SKGO_NATIVE_CAPTURE_RECEIPT", "captured-env-412")
	t.Setenv("SKGO_NATIVE_CAPTURE_OWNER", filepath.Join(work, "owner.lock"))
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
	write("go.mod", strings.ReplaceAll(strings.ReplaceAll(string(mod), "module github.com/tylergannon/skgo/example", "module example.com/nativehost"), "=> ../", "=> "+filepath.ToSlash(root))+"\nrequire example.com/replaced v0.0.0\n")
	sums, err := os.ReadFile(filepath.Join(root, "example/go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	write("go.sum", string(sums))
	// The same retained inputs must select this sibling during analysis, native
	// host build and subsequent body-edit preparation. No module replace exists.
	shared := filepath.Join(work, "shared")
	if err := os.MkdirAll(shared, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "go.mod"), []byte("module example.com/shared\ngo 1.27.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "shared.go"), []byte("package shared\nconst Receipt=\"workspace-912\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	replacementDependency := filepath.Join(work, "replacement")
	if err := os.MkdirAll(replacementDependency, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacementDependency, "go.mod"), []byte("module example.com/replaced\ngo 1.27.2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacementDependency, "value.go"), []byte("package replaced\nconst Receipt=\"workspace-replace-314\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(work, "go.work")
	if err := os.WriteFile(workspace, []byte("go 1.27.2\nuse (\n./app\n./shared\n)\nreplace example.com/replaced v0.0.0 => ./replacement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", workspace)
	write("other/config.go", "package other\ntype FetchConfig struct{ Value string }\n")
	write("web/package.json", `{"name":"native-host-contract","type":"module"}`)
	write("web/src/components/Notice.svelte", `<p>first frontend</p>`)
	write("locals/locals.go", "package locals\ntype Locals struct{}")
	source := `package orders
import("fmt";"net/http";"github.com/tylergannon/skgo")
var initial=7
func get(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Literal","native-hook");fmt.Fprintf(w,"literal-4101:%d",initial)}
var _=skgo.GET(get)
`
	write("web/src/routes/orders/server.go", source)
	write("web/src/routes/second/server.go", strings.ReplaceAll(strings.Replace(source, "package orders", "package second", 1), "4101", "8808"))
	write("assets/one.txt", "asset-one")
	write("assets/keep.txt", "keep-member")
	write("server.go", `package nativehost
import("embed";"io/fs";"net/http";"os";"syscall";"example.com/shared";"example.com/replaced";"example.com/nativehost/other";"github.com/tylergannon/skgo";generated "example.com/nativehost/generated")
//go:embed assets
var resources embed.FS
var ownerLock *os.File
func NewHandler(_ fs.FS,origin string)(http.Handler,error){
if ownerLock==nil{lock,err:=os.OpenFile(os.Getenv("SKGO_NATIVE_CAPTURE_OWNER"),os.O_CREATE|os.O_RDWR,0600);if err!=nil{return nil,err};if err:=syscall.Flock(int(lock.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{lock.Close();return nil,err};ownerLock=lock}
unrelated:=other.FetchConfig{Value:"unrelated-413"};type FetchAlias = skgo.FetchConfig;fetch:=FetchAlias{Origin:origin};endpoints,e:=skgo.NewEndpoints(skgo.EndpointConfig{Origin:origin,Routes:[]skgo.ManifestRoute{{ID:"/orders",Pattern:"^/orders/?$",Endpoint:&skgo.ManifestEndpoint{Methods:[]string{"GET"}}},{ID:"/second",Pattern:"^/second/?$",Endpoint:&skgo.ManifestEndpoint{Methods:[]string{"GET"}}}}},generated.Endpoints()...);if e!=nil{return nil,e};return fetch.Intercept(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Workspace",shared.Receipt);w.Header().Set("X-Workspace-Replacement",replaced.Receipt);w.Header().Set("X-Captured-Environment",os.Getenv("SKGO_NATIVE_CAPTURE_RECEIPT"));w.Header().Set("X-Unrelated",unrelated.Value);data,_:=resources.ReadFile("assets/one.txt");w.Header().Set("X-Resource",string(data));w.Header().Set("X-App-Hook","preserved");endpoints.Intercept(http.NotFoundHandler()).ServeHTTP(w,r)})),nil}
`)

	project, err := gen.OpenProject(gen.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/nativehost/locals", LocalsType: "Locals", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer project.Close()
	prepare := func() *gen.ProjectBuild {
		t.Helper()
		b, e := project.Prepare(ctx)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { b.Cancel(); b.Inputs.Close() })
		return b
	}
	first := prepare()
	t.Setenv("SKGO_NATIVE_CAPTURE_RECEIPT", "ambient-env-881")
	host, err := BuildHost(ctx, HostOptions{Inputs: first.Inputs, WorkDir: filepath.Join(work, "first"), Origin: "http://localhost", DistDir: work, Output: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	t.Setenv("SKGO_NATIVE_CAPTURE_RECEIPT", "captured-env-412")
	check := func(h *Host, want string) {
		t.Helper()
		response, e := http.Get(h.URL + "/orders")
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		if e != nil || response.StatusCode != 200 || string(body) != want || response.Header.Get("X-App-Hook") != "preserved" || response.Header.Get("X-Literal") != "native-hook" || response.Header.Get("X-Resource") != "asset-one" || response.Header.Get("X-Workspace") != "workspace-912" || response.Header.Get("X-Unrelated") != "unrelated-413" || response.Header.Get("X-Workspace-Replacement") != "workspace-replace-314" || response.Header.Get("X-Captured-Environment") != "captured-env-412" {
			t.Fatalf("real handler got status=%d body=%q headers=%v err=%v; want %q", response.StatusCode, body, response.Header, e, want)
		}
	}
	check(host, "literal-4101:7")
	response, err := http.Get(host.URL + "/second")
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(secondBody) != "literal-8808:7" {
		t.Fatalf("second native package body%q err%v", secondBody, err)
	}
	write("web/src/components/Notice.svelte", `<p>changed frontend</p>`)
	if _, _, err := project.Observe(ctx, filepath.Join(app, "web/src/components/Notice.svelte")); err != nil {
		t.Fatal(err)
	}
	frontend := prepare()
	if u, err := host.Prepare(ctx, frontend.Inputs, filepath.Join(work, "frontend")); err != nil || u != nil {
		t.Fatalf("frontend-only edit compiled Go: %v, %v", u, err)
	}

	edit := func(next string) *gen.ProjectBuild {
		t.Helper()
		write("web/src/routes/orders/server.go", next)
		if _, _, e := project.Observe(ctx, filepath.Join(app, "web/src/routes/orders/server.go")); e != nil {
			t.Fatal(e)
		}
		return prepare()
	}
	for n, literal := range []string{"5202", "6303"} {
		b := edit(strings.Replace(source, "4101", literal, 1))
		u, e := host.Prepare(ctx, b.Inputs, filepath.Join(work, fmt.Sprintf("candidate-%d", n)))
		if e != nil {
			t.Fatal(e)
		}
		if e = b.Commit(func(*gen.CompilerInputs) error { return host.Install(ctx, u) }); e != nil {
			t.Fatal(e)
		}
		check(host, "literal-"+literal+":7")
	}

	write("web/src/routes/second/server.go", strings.ReplaceAll(strings.Replace(source, "package orders", "package second", 1), "4101", "9909"))
	if _, _, err := project.Observe(ctx, filepath.Join(app, "web/src/routes/second/server.go")); err != nil {
		t.Fatal(err)
	}
	secondRevision := prepare()
	secondUpdate, err := host.Prepare(ctx, secondRevision.Inputs, filepath.Join(work, "second-package"))
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Install(ctx, secondUpdate); err != nil {
		t.Fatal(err)
	}
	check(host, "literal-6303:7")
	response, err = http.Get(host.URL + "/second")
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(secondBody) != "literal-9909:7" {
		t.Fatalf("second package update body%q err%v", secondBody, err)
	}

	for _, resource := range []struct{ name, body string }{{"assets/one.txt", "asset-changed"}, {"assets/new.txt", "new-member"}, {"assets/one.txt", ""}} {
		if resource.body == "" {
			if err := os.Remove(filepath.Join(app, resource.name)); err != nil {
				t.Fatal(err)
			}
		} else {
			write(resource.name, resource.body)
		}
		if _, _, err := project.Observe(ctx, filepath.Join(app, resource.name)); err != nil {
			t.Fatal(err)
		}
		changedResource := prepare()
		_, err := host.Prepare(ctx, changedResource.Inputs, filepath.Join(work, "resource"))
		var rebuild *CompleteBuild
		if !errors.As(err, &rebuild) {
			t.Fatalf("embedded resource admitted without rebuilding: %s, %v", resource.name, err)
		}
		if resource.name == "assets/one.txt" {
			write(resource.name, "asset-one")
		} else if err := os.Remove(filepath.Join(app, resource.name)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := project.Observe(ctx, filepath.Join(app, resource.name)); err != nil {
			t.Fatal(err)
		}
	}
	b := edit(strings.Replace(source, "initial=7", "initial=9", 1))
	if _, err = host.Prepare(ctx, b.Inputs, filepath.Join(work, "structural")); err == nil {
		t.Fatal("initializer edit admitted natively")
	} else {
		var full *CompleteBuild
		if !errors.As(err, &full) {
			t.Fatal(err)
		}
	}
	replacement, err := CompileHost(ctx, HostOptions{Inputs: b.Inputs, WorkDir: filepath.Join(work, "replacement"), Origin: "http://localhost", DistDir: work, Output: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if replacement.PID != 0 {
		t.Fatal("compilation started the replacement application")
	}
	check(host, "literal-6303:7")
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if replacement.PID == host.PID {
		t.Fatal("replacement PID unchanged")
	}
	check(replacement, "literal-4101:9")
	repaired := edit(strings.Replace(strings.Replace(source, "initial=7", "initial=9", 1), "4101", "7407", 1))
	u, err := replacement.Prepare(ctx, repaired.Inputs, filepath.Join(work, "resumed"))
	if err != nil {
		t.Fatal(err)
	}
	if err = replacement.Install(ctx, u); err != nil {
		t.Fatal(err)
	}
	check(replacement, "literal-7407:9")
	// Replacement must retire the actual OS process and listener, including
	// after retained compilation allocated its private source view.
	retired := func(h *Host) {
		t.Helper()
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-h.Done():
		default:
			t.Fatal("closed native process has not exited")
		}
		if h.cmd.ProcessState == nil {
			t.Fatal("native child was not reaped")
		}
		for _, pipe := range []*os.File{h.commands, h.replies} {
			if _, err := pipe.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("native control pipe remains open: %v", err)
			}
		}
		connection, err := net.DialTimeout("tcp", strings.TrimPrefix(h.URL, "http://"), 200*time.Millisecond)
		if err == nil {
			connection.Close()
			t.Fatal("retired native listener still accepts connections")
		}
	}
	retired(host)
	retired(replacement)
}
