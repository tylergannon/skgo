// Package devrender_test drives the example's development server: `vp dev`
// behind, Go in front, the way `just dev` runs them. Go renders every document
// out of the modules the running vite server transforms, so what these tests
// prove is the dev arm of example.NewHandler — the one a browser run in dev
// mode otherwise has to reach.
//
// TestMain starts one `vp dev` for the whole package, on a copy of example/web
// so that the source edit below never touches the developer's tree, and every
// test renders through the one handler built over it.
package devrender_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/web"
)

// origin is the origin the handler is configured with. Nothing listens on it:
// every request below is served in-process.
const origin = "http://127.0.0.1:8080"

var (
	// devServer is the running `vp dev`'s URL.
	devServer string
	// webRoot is the vite root that server serves: a copy of example/web.
	webRoot string
	// handler is example.NewHandler's dev arm over devServer.
	handler http.Handler
	// viteLog is everything `vp dev` printed, for a failure to show.
	viteLog bytes.Buffer
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "skgo-devrender-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)

	webRoot = filepath.Join(dir, "web")
	if err := copyWebRoot(filepath.Join("..", "web"), webRoot); err != nil {
		fmt.Fprintf(os.Stderr, "copying example/web: %v\n", err)
		return 1
	}
	port, err := freePort()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	devServer = "http://127.0.0.1:" + strconv.Itoa(port)

	// The project-local vp, as `just dev` runs it: kit checks the SSR
	// environment with instanceof against the project's own vite. A missing
	// toolchain fails here, loudly; it is never a reason to skip.
	vite := exec.Command("mise", "x", "--", filepath.Join("node_modules", ".bin", "vp"),
		"dev", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--strictPort")
	vite.Dir = webRoot
	vite.Stdout, vite.Stderr = &viteLog, &viteLog
	// Its own process group, so stopping it stops the node process mise
	// started rather than only mise.
	vite.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := vite.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "starting vp dev: %v\n", err)
		return 1
	}
	defer func() {
		_ = syscall.Kill(-vite.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = vite.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-vite.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}()

	// NewHandler waits for the dev server to answer before it builds
	// anything, the same wait `just dev` relies on.
	h, mode, err := example.NewHandler(dist(), devServer, origin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembling the dev stack against %s: %v\n--- vp dev ---\n%s", devServer, err, viteLog.String())
		return 1
	}
	if mode != "dev" {
		fmt.Fprintf(os.Stderr, "mode = %q, want dev\n", mode)
		return 1
	}
	handler = h
	code := m.Run()
	if code != 0 {
		fmt.Fprintf(os.Stderr, "--- vp dev ---\n%s", viteLog.String())
	}
	return code
}

func dist() fs.FS {
	sub, err := fs.Sub(web.Build, "build")
	if err != nil {
		panic(err)
	}
	return sub
}

// TestEveryRouteRendersThroughTheDevHandler asks the dev handler for a
// concrete URL of every page route in kit's live dev graph.
//
// In dev nothing is bundled: each document is rendered from modules pulled one
// at a time out of the running vite server, so a route whose modules the
// engine cannot evaluate fails here and nowhere else short of a browser. And
// in dev, kit's own module runner is what would call the generated
// `.remote.ts` and `+*.server.ts` stubs, which throw "implemented in Go" — so
// that text in a document is kit having answered instead of Go.
func TestEveryRouteRendersThroughTheDevHandler(t *testing.T) {
	manifest, err := skgo.ReadDevManifest(dist(), devServer)
	if err != nil {
		t.Fatalf("reading kit's live dev manifest: %v", err)
	}
	session := businesslogic.Default.SignIn("ada")

	pages, rendered, errored := 0, 0, 0
	for _, route := range manifest.Routes {
		if route.Page == nil {
			continue
		}
		pages++
		path := samplePath(t, route.ID)
		if route.ID == "/actions/profiles/[profile]" {
			path = "/actions/profiles/ada"
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html")
		req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		body := rec.Body.String()

		if strings.Contains(body, "implemented in Go") {
			t.Errorf("route %s: GET %s ran a generated stub, so kit answered rather than Go", route.ID, path)
		}

		if want, deliberate := deliberateFailures[route.ID]; deliberate {
			errored++
			if rec.Code != want.status {
				t.Errorf("route %s: GET %s returned %d, want %d", route.ID, path, rec.Code, want.status)
			}
			if want.location != "" {
				if got := rec.Header().Get("Location"); got != want.location {
					t.Errorf("route %s: GET %s sent Location %q, want %q", route.ID, path, got, want.location)
				}
				continue
			}
			if !strings.Contains(body, want.says) {
				t.Errorf("route %s: GET %s does not say %q", route.ID, path, want.says)
			}
			continue
		}

		if rec.Code != http.StatusOK {
			t.Errorf("route %s: GET %s returned %d, want 200\n%s", route.ID, path, rec.Code, body)
			continue
		}
		ssr := true
		for _, index := range route.Page.Branch() {
			if index >= 0 && index < len(manifest.SSR.Nodes) {
				if v := manifest.SSR.Nodes[index].SSR; v != nil {
					ssr = *v
				}
			}
		}
		if !ssr {
			// A branch that turns server rendering off is vite's to answer
			// with kit's shell, which carries no page markup.
			if strings.Contains(body, `data-testid="app-nav"`) {
				t.Errorf("route %s turns SSR off: GET %s rendered page content", route.ID, path)
			}
			continue
		}
		if !strings.Contains(body, `data-testid="app-nav"`) {
			t.Errorf("route %s: GET %s carries no rendered markup from the root layout:\n%s", route.ID, path, body)
			continue
		}
		rendered++
	}
	if pages == 0 {
		t.Fatal("kit's dev manifest lists no page routes, so this test asserted nothing")
	}
	if rendered == 0 {
		t.Fatal("not one page route was rendered, so this test asserted nothing about dev rendering")
	}
	if errored != len(deliberateFailures) {
		t.Errorf("%d of the %d routes this app fails on purpose were visited", errored, len(deliberateFailures))
	}

	// A rendered page carries what a Go function answered while the document
	// was built, which kit's own dev server, running the throwing stub, cannot.
	rec := get("/items/42")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Widget 42") {
		t.Errorf("GET /items/42 in dev: status %d, and the document does not carry Go's \"Widget 42\"", rec.Code)
	}
}

// TestASourceEditReachesTheNextDevDocument is the dev server's reason to
// exist: an edit to a component changes the next document Go renders, with
// nothing rebuilt and nothing restarted. The heading is written into the
// test, before and after, rather than read off a response.
func TestASourceEditReachesTheNextDevDocument(t *testing.T) {
	const (
		file   = "src/routes/+page.svelte"
		source = `<h1 data-testid="title">Home</h1>`
		edited = `<h1 data-testid="title">Home, edited while running</h1>`
	)
	// Svelte adds its scoping class to the rendered tag, so the document is
	// matched on the heading's text inside the title element.
	heading := func(text string) *regexp.Regexp {
		return regexp.MustCompile(`<h1 data-testid="title"[^>]*>` + regexp.QuoteMeta(text) + `</h1>`)
	}
	original, changed := heading("Home"), heading("Home, edited while running")
	if body := get("/").Body.String(); !original.MatchString(body) {
		t.Fatalf("GET / did not carry the heading Home before the edit:\n%s", body)
	}

	path := filepath.Join(webRoot, filepath.FromSlash(file))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(raw, []byte(source)); n != 1 {
		t.Fatalf("%s carries %d copies of %s; the edit has to be unambiguous", file, n, source)
	}
	if err := os.WriteFile(path, bytes.Replace(raw, []byte(source), []byte(edited), 1), 0o644); err != nil {
		t.Fatal(err)
	}

	// The edit has to reach vite's watcher before the engine can be told to
	// drop the module, so this polls; a Go that never picks the edit up never
	// satisfies it.
	deadline := time.Now().Add(20 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		body = get("/").Body.String()
		if changed.MatchString(body) {
			if original.MatchString(body) {
				t.Fatalf("GET / carries both the edited and the original heading:\n%s", body)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("GET / still does not carry the edited heading 20s after the edit:\n%s", body)
}

func get(path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// deliberateFailures is what this app does wrong on purpose, written out
// rather than read back off a response. It is the dev half of the table in
// example/server_test.go: the same statuses and the same words, because a
// document rendered from vite's modules has to fail the way the built one
// does.
var deliberateFailures = map[string]struct {
	status   int
	location string
	says     string
}{
	"/account/statement": {status: 402, says: "Your account is in arrears"},
	"/error/expected":    {status: 418, says: "This page is a teapot"},
	"/error/unexpected":  {status: 500, says: "Something went wrong on our end."},
	"/error/command":     {status: 500, says: "Internal Error"},
	"/error/server-only": {status: 500, says: "Internal Error"},
	"/error/boundary":    {status: 409, says: "The sensor is being calibrated"},
	"/error/redirect":    {status: 307, location: "/about"},
	"/error/render":      {status: 500, says: "Something went wrong on our end."},
}

var (
	restSegment  = regexp.MustCompile(`^\[\.\.\.[\w-]+\]$`)
	paramSegment = regexp.MustCompile(`^\[[\w-]+\]$`)
	groupSegment = regexp.MustCompile(`^\([^)]+\)$`)
)

// samplePath turns a route id into a URL that matches it.
func samplePath(t *testing.T, id string) string {
	t.Helper()
	if id == "/" {
		return "/"
	}
	var out []string
	for _, segment := range strings.Split(strings.TrimPrefix(id, "/"), "/") {
		switch {
		case groupSegment.MatchString(segment):
		case restSegment.MatchString(segment):
			out = append(out, "deep", "rest", "path")
		case paramSegment.MatchString(segment):
			out = append(out, "sample")
		case !strings.ContainsAny(segment, "[]()"):
			out = append(out, segment)
		default:
			t.Fatalf("route %s has segment %q, which this test does not know how to visit; teach it before adding the route", id, segment)
		}
	}
	return "/" + strings.Join(out, "/")
}

// freePort asks the kernel for a port nothing is listening on. It is never
// 5173 or 8080, the ports `just dev` and `just serve` use.
func freePort() (int, error) {
	for {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if port != 5173 && port != 8080 {
			return port, nil
		}
	}
}

// copyWebRoot copies the vite root without what the build and the install
// write, and gives the copy its own node_modules whose entries point at the
// installed ones.
//
// node_modules cannot be a single symlink: `node_modules/$app` holds the
// tsconfig kit writes with relative rootDirs, which through a link would name
// the original tree, so it is copied. Vite's own caches (`.vite`,
// `.vite-temp`) are left out, so this server never writes into the one the
// developer's `just dev` uses.
func copyWebRoot(src, dst string) error {
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	skip := map[string]bool{"node_modules": true, "build": true, ".svelte-kit": true}
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		if skip[rel] {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
	if err != nil {
		return err
	}
	modules := filepath.Join(abs, "node_modules")
	entries, err := os.ReadDir(modules)
	if err != nil {
		return fmt.Errorf("%w (run `just install`)", err)
	}
	if err := os.MkdirAll(filepath.Join(dst, "node_modules"), 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, ".vite"):
			continue
		case name == "$app":
			if err := copyDir(filepath.Join(modules, name), filepath.Join(dst, "node_modules", name)); err != nil {
				return err
			}
		default:
			if err := os.Symlink(filepath.Join(modules, name), filepath.Join(dst, "node_modules", name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, info.Mode().Perm())
}
