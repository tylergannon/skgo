package newapp_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/newapp"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// scaffoldVersion is the version this checkout is published under, into the
// module proxy the scaffolded project fetches skgo from. It is a prerelease of
// v0.0.0 so that it can never be confused with a released one.
// It carries the run's start time because the go command treats the module
// cache as immutable and indexes it by version: publish twice under one version
// and the second run compiles the first run's source, silently. That is not
// hypothetical — this test was green for a checkout it had never compiled until
// a change to the adapter made the stale copy fail out loud.
var scaffoldVersion = "v0.0.0-scaffoldtest" + strconv.FormatInt(time.Now().UnixNano(), 10)

// scaffoldVersionPrefix is what every run of this test publishes under, so that
// a run can clear the ones before it out of the module cache rather than
// leaving a copy of the checkout there for good.
const scaffoldVersionPrefix = "v0.0.0-scaffoldtest"

// greeted is the name the test sends to the app's command. It is the whole
// point of the fixture: the value the page ends up showing has to be one the
// test supplied, not one read back out of the app before the call.
const greeted = "scaffold-acceptance"

// TestAScaffoldedProjectBuildsAndServes is the mission in one test: `skgo new`
// into an empty directory, one build gesture, and a binary that answers.
//
// It is the only place skgo is exercised the way anybody but this repository
// will ever use it. Everything else here runs inside a go.work with skgo as a
// writable checkout, where a generator that writes next to its own source, or a
// go:generate that needs the library to be editable, works perfectly and is
// broken for every real consumer. So this test publishes the checkout into a
// throwaway module proxy and lets the new project fetch it from the module
// cache — read-only, no replace directive, no workspace.
//
// It is slow, and it needs a real toolchain: Go, mise, and the node it
// installs. That is not a reason to make it skippable. A scaffold nobody has
// built is a scaffold nobody knows works, and a skipped test reports the same
// exit code as a passing one.
func TestAScaffoldedProjectBuildsAndServes(t *testing.T) {
	root := t.TempDir()
	proxy := publish(t, filepath.Join(root, "proxy"))
	dir := filepath.Join(root, "myapp")
	port := freePort(t)
	origin := fmt.Sprintf("http://127.0.0.1:%d", port)

	// The adapter is an npm package now, and nothing is on the registry yet, so
	// the project installs the tarball `pnpm pack` produces from this
	// checkout's package root. That tarball is the artifact a release would
	// upload: if it is missing a file the adapter needs, this is where that
	// shows up, rather than in a relative import that only works from inside
	// this repository.
	adapter := packAdapter(t, root)

	if err := newapp.Create(newapp.Options{
		Dir:         dir,
		Origin:      origin,
		SkgoVersion: scaffoldVersion,
		AdapterSpec: "file:" + adapter,
		Logf:        t.Logf,
	}); err != nil {
		t.Fatalf("scaffolding the project: %v", err)
	}

	// Nothing in the generated project may point back at this checkout: a
	// replace directive or a workspace would make the whole test a
	// reformulation of `go test ./...`.
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gomod), "replace") {
		t.Fatalf("the generated go.mod has a replace directive:\n%s", gomod)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
		t.Fatal("the generated project has a go.work; it must consume skgo as an ordinary dependency")
	}

	env := append(os.Environ(),
		// The file proxy first, the real one behind it for polytype and the
		// rest. A file proxy answers 404 for what it does not have, and the go
		// command falls through on 404.
		"GOPROXY=file://"+filepath.ToSlash(proxy)+",https://proxy.golang.org,direct",
		// A developer whose GOPRIVATE covers github.com/tylergannon/* — which
		// is an ordinary thing to have — sends every module under it straight
		// to git and never asks a proxy at all. That is right for them and
		// fatal here, where the version being fetched exists in no repository.
		"GOPRIVATE=",
		"GONOPROXY=none",
		"GONOSUMDB=none",
		// scaffoldVersion is not in any checksum database, and never will be.
		"GOSUMDB=off",
		"GOFLAGS=-mod=mod",
		"GOWORK=off",
	)

	// Nothing about the adapter is vendored. It is a devDependency and an
	// import, the way every other kit adapter is, so nothing adapter-shaped
	// ever appears in the developer's own directory.
	planted := []string{
		filepath.Join(dir, "web", "skgo-adapter.js"),
		filepath.Join(dir, "web", "skgo-adapter"),
	}
	for _, at := range planted {
		if _, err := os.Stat(at); err == nil {
			t.Fatalf("`skgo new` wrote %s; the adapter is installed, not scaffolded", at)
		}
	}

	// mise refuses to run a config file it has not been told to trust, and a
	// developer answers that prompt once. The test has no prompt to answer.
	run(t, dir, env, "mise", "trust", "--yes")

	// The gesture the README documents, not a private reimplementation of it.
	// If `mise run build` stops being the one thing a developer types, this
	// test is the thing that notices.
	run(t, dir, env, "mise", "run", "build")

	// The type check is the second thing a developer types, and the scaffold is
	// where a kit-3 rule they cannot see (a `#lib` import of a `.ts` module
	// needs its extension) would first bite. It runs kit's sync itself, so it
	// does not depend on a build having happened.
	run(t, filepath.Join(dir, "web"), env, "mise", "x", "--", "vp", "run", "check")

	// And the build did not put one there either. A file that materialises in
	// the vite root after a build is still a file the developer did not ask
	// for, and it is what this milestone exists to remove.
	for _, at := range planted {
		if _, err := os.Stat(at); err == nil {
			t.Fatalf("the build gesture wrote %s into the developer's tree", at)
		}
	}

	// The build says which adapter made it. The fingerprint is computed here
	// from the checkout's package root — the same bytes `pnpm pack` shipped —
	// rather than asked of the code that does the checking, and the adapter
	// that wrote the manifest computed its own from the installed copy. Two
	// hashes over two copies of the same files, so they agree only if the
	// tarball really is what this module embeds.
	stamp := manifestStamp(t, dir)
	if want := fingerprintOf(t, filepath.Join(checkoutRoot(t), "internal", "adapter")); stamp.Adapter != want {
		t.Fatalf("the frontend was built by adapter %q; the checkout's adapter is %q",
			stamp.Adapter, want)
	}
	if want := packageVersion(t, filepath.Join(checkoutRoot(t), "internal", "adapter")); stamp.Skgo != want {
		t.Fatalf("the manifest names skgo %q; the package that built it is version %q",
			stamp.Skgo, want)
	}

	binary := filepath.Join(dir, "bin", "myapp")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("the build gesture did not produce %s: %v", binary, err)
	}

	// The route directory is the developer's, and the generator adds exactly
	// three things beside a page: the stub kit compiles, the types its
	// callers see, and the registration that names the handlers. The root
	// also carries the go.mod that fences the route tree off from `go build
	// ./...`. Nothing of polytype's — no marker, no schema, no build-tagged
	// Go — lands anywhere in it.
	routes := filepath.Join(dir, "web", "src", "routes")
	want := []string{
		"+error.svelte", "+layout.svelte", "+page.svelte",
		"about/+page.svelte", "go.mod",
		"hello.remote.go", "hello.remote.ts", "skgo_remotes_gen.go", "types.ts",
	}
	if got := filesUnder(t, routes); !slices.Equal(got, want) {
		t.Fatalf("after the build gesture %s holds:\n  %s\nwant:\n  %s",
			routes, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	serve(t, binary, port)
	client := &http.Client{Timeout: 20 * time.Second}
	awaitServer(t, client, origin)

	t.Run("it renders the app in Go", func(t *testing.T) {
		body := getOK(t, client, origin+"/")
		// The home page's markup, with the values Go answered already in it:
		// the app's own name and the Go that built the binary. Neither can be
		// in the document unless the render happened in this process — kit's
		// SPA fallback carries no h1 at all.
		for _, want := range []string{
			`<h1 data-testid="title">myapp</h1>`,
			`Served by go1.`,
			`<strong data-testid="greetings">0</strong>`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("GET / was not rendered in Go; it lacks %q:\n%s", want, firstLines(body, 40))
			}
		}
		// And it still boots kit's client from _app/, so the page hydrates.
		if !strings.Contains(body, "/_app/immutable/") {
			t.Fatalf("GET / carries no client script:\n%s", firstLines(body, 40))
		}
		// A deep link is its own render, not a shared fallback document.
		about := getOK(t, client, origin+"/about")
		if !strings.Contains(about, `<h1 data-testid="title">About</h1>`) {
			t.Fatalf("GET /about was not rendered in Go:\n%s", firstLines(about, 40))
		}
	})

	t.Run("Go answers the remote functions", func(t *testing.T) {
		ids := remoteIDs(t, dir)

		// The counter starts where the scenario says it starts: this process
		// has served no command yet.
		before := status(t, client, origin, ids["status"])
		if before.Greetings != 0 || before.LastGreeting != "" {
			t.Fatalf("a freshly started server reports %d greeting(s), last %q; want 0 and none",
				before.Greetings, before.LastGreeting)
		}
		if before.Name != "myapp" || !strings.HasPrefix(before.GoVersion, "go1.") {
			t.Fatalf("the query answered %+v; want the app's own name and the Go that built it", before)
		}

		// One command, from the test, with the name the test chose.
		greet(t, client, origin, ids["greet"])

		after := status(t, client, origin, ids["status"])
		if after.Greetings != 1 || after.LastGreeting != greeted {
			t.Fatalf("after one command the server reports %d greeting(s), last %q; want 1 and %q",
				after.Greetings, after.LastGreeting, greeted)
		}
	})

	t.Run("a command from the wrong origin is refused with an explanation", func(t *testing.T) {
		ids := remoteIDs(t, dir)
		req := commandRequest(t, origin, ids["greet"])
		req.Header.Set("Origin", "http://127.0.0.1:1")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status %d; want 403", resp.StatusCode)
		}
		// The bare 403 kit and skgo answer with says nothing a developer can
		// act on, and this is the failure a new project is likeliest to hit.
		// The body has to name both origins and the file that decides.
		for _, want := range []string{origin, "http://127.0.0.1:1", "mise.toml", "ORIGIN"} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("the refusal does not mention %q:\n%s", want, body)
			}
		}
	})
}

// Status mirrors the template's own wire type. It is written out here rather
// than imported so the test asserts against a shape it states, not against
// whatever the template currently declares.
type Status struct {
	Name         string `json:"name"`
	GoVersion    string `json:"goVersion"`
	Greetings    int    `json:"greetings"`
	LastGreeting string `json:"lastGreeting"`
}

// remoteIDs reads the `<hash>/<name>` ids out of the frontend build, keyed by
// function name. The hash is kit's, derived from the module's path, so there is
// nowhere else to learn it; the test uses it as an address and never as an
// expectation.
func remoteIDs(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "web", "build", "skgo.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, id := range manifest.Remotes {
		_, name, _ := strings.Cut(id, "/")
		ids[name] = id
	}
	for _, want := range []string{"status", "greet"} {
		if ids[want] == "" {
			t.Fatalf("the build declares %v; the template's %s is missing", manifest.Remotes, want)
		}
	}
	return ids
}

func status(t *testing.T, client *http.Client, origin, id string) Status {
	t.Helper()
	resp, err := client.Get(origin + "/_app/remote/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("querying %s: status %d", id, resp.StatusCode)
	}
	var s Status
	if err := json.Unmarshal(result(t, resp), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func greet(t *testing.T, client *http.Client, origin, id string) {
	t.Helper()
	req := commandRequest(t, origin, id)
	req.Header.Set("Origin", origin)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("commanding %s: status %d: %s", id, resp.StatusCode, body)
	}
}

// commandRequest is the POST kit's client makes: a devalue-encoded argument in
// a base64url payload. The encoder is skgo's own, and it is checked against
// kit's goldens in internal/remotearg; what this test asserts is what the app
// does with the argument, not how it travelled.
func commandRequest(t *testing.T, origin, id string) *http.Request {
	t.Helper()
	payload, err := remotearg.StringifyCommandArg(greeted)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, origin+"/_app/remote/"+id, strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

// result pulls a remote function's value out of the envelope kit's client
// reads: `{"type":"result","data":"<devalue>"}`, whose `_` holds the value.
func result(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	var envelope struct {
		Type  string          `json:"type"`
		Data  string          `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Type != "result" {
		t.Fatalf("the server answered %s: %s", envelope.Type, envelope.Error)
	}
	value, err := devalue.Parse(envelope.Data, nil)
	if err != nil {
		t.Fatalf("parsing %q: %v", envelope.Data, err)
	}
	object, ok := value.(*devalue.Object)
	if !ok {
		t.Fatalf("the response is a %T, not an object", value)
	}
	inner, ok := object.Get("_")
	if !ok {
		t.Fatalf("the response has no `_`: %s", envelope.Data)
	}
	raw, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func getOK(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d: %s", url, resp.StatusCode, body)
	}
	return string(body)
}

// serve starts the built binary and keeps it running for the rest of the test.
func serve(t *testing.T, binary string, port int) {
	t.Helper()
	cmd := exec.Command(binary, "--listen", fmt.Sprintf("127.0.0.1:%d", port))
	var log strings.Builder
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting %s: %v", binary, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		if t.Failed() {
			t.Logf("the server said:\n%s", log.String())
		}
	})
}

func awaitServer(t *testing.T, client *http.Client, origin string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(origin + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the binary never answered on %s", origin)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func run(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Fatalf("%s is not on PATH, so this test cannot run — which is not the same as passing: %v", name, err)
	}
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	t.Logf("%s %s:\n%s", name, strings.Join(args, " "), out)
}

// publish writes this checkout into a module proxy on disk, so the scaffolded
// project can require it by version like any other dependency.
//
// The alternative — a replace directive, or a go.work — is the configuration
// this test exists to avoid: skgo lands in the module cache read-only, and
// anything the generator does that needs its own source tree to be writable
// fails here and only here.
func publish(t *testing.T, dir string) string {
	t.Helper()
	source := checkoutRoot(t)
	mv := module.Version{Path: "github.com/tylergannon/skgo", Version: scaffoldVersion}
	purgeFromModuleCache(t, mv.Path)

	at := filepath.Join(dir, filepath.FromSlash(mv.Path), "@v")
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}

	archive, err := os.Create(filepath.Join(at, scaffoldVersion+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if err := zip.Create(archive, mv, moduleFiles(t, source)); err != nil {
		t.Fatalf("packaging the checkout as a module: %v", err)
	}

	gomod, err := os.ReadFile(filepath.Join(source, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, contents []byte) {
		if err := os.WriteFile(filepath.Join(at, name), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(scaffoldVersion+".mod", gomod)
	write(scaffoldVersion+".info", []byte(fmt.Sprintf(`{"Version":%q,"Time":%q}`,
		scaffoldVersion, time.Now().UTC().Format(time.RFC3339))))
	write("list", []byte(scaffoldVersion+"\n"))
	return dir
}

// purgeFromModuleCache removes what earlier runs of this test left behind.
//
// Every run publishes under a fresh version, so nothing here is load-bearing
// for correctness — it just keeps the module cache from accumulating one copy
// of the checkout per run. The cache is deliberately read-only, so the
// permissions come back first.
func purgeFromModuleCache(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatalf("locating the module cache: %v", err)
	}
	cache := strings.TrimSpace(string(out))
	if cache == "" {
		return
	}

	remove := func(name string) {
		_ = filepath.WalkDir(name, func(p string, d fs.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
		_ = os.RemoveAll(name)
	}

	dir := filepath.Dir(filepath.Join(cache, filepath.FromSlash(path)))
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "skgo@"+scaffoldVersionPrefix) {
			remove(filepath.Join(dir, entry.Name()))
		}
	}

	downloads := filepath.Join(cache, "cache", "download", filepath.FromSlash(path), "@v")
	entries, _ = os.ReadDir(downloads)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), scaffoldVersionPrefix) {
			remove(filepath.Join(downloads, entry.Name()))
		}
	}
}

// moduleFiles is what a consumer of skgo gets: the library, the command, the
// internal packages the two are built from, and nothing else. The example app
// is a module of its own and ephemeral/ is working material; neither is part of
// what anyone imports, and both hold symbolic links a module zip cannot carry.
func moduleFiles(t *testing.T, root string) []zip.File {
	t.Helper()
	var files []zip.File
	for _, top := range []string{".", "cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if top == "." && rel != "." {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			if top == "." && !strings.HasSuffix(rel, ".go") && rel != "go.mod" && rel != "go.sum" {
				return nil
			}
			files = append(files, diskFile{root: root, rel: path.Clean(filepath.ToSlash(rel))})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

type diskFile struct {
	root string
	rel  string
}

func (f diskFile) Path() string                 { return f.rel }
func (f diskFile) Lstat() (os.FileInfo, error)  { return os.Lstat(f.abs()) }
func (f diskFile) Open() (io.ReadCloser, error) { return os.Open(f.abs()) }
func (f diskFile) abs() string {
	return filepath.Join(f.root, filepath.FromSlash(f.rel))
}

// filesUnder lists every regular file below dir, relative to it and sorted,
// so a directory's contents can be stated exactly.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// checkoutRoot is the module this test is compiled from.
func checkoutRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the checkout this test was built from")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("%s is not the module root: %v", root, err)
	}
	return root
}

// fingerprintOf is how skgo names an adapter: the first 12 hex digits of the
// SHA-256 of everything it is made of, before `skgo generate` stamps it — the
// entry the vite config imports, then each runtime file beside it under its own
// slash-separated path, in sorted order.
//
// It is spelled out here rather than asked of `adapter.Fingerprint`, because a
// test that asks the code under test what it expects passes however wrong that
// code is.
func fingerprintOf(t *testing.T, dir string) string {
	t.Helper()
	sum := sha256.New()
	sum.Write(readFile(t, filepath.Join(dir, "skgo-adapter.js")))

	filesDir := filepath.Join(dir, "skgo-adapter")
	var names []string
	if err := filepath.WalkDir(filesDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			names = append(names, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatalf("%s holds no runtime files; the adapter is more than one file", filesDir)
	}

	relative := make([]string, len(names))
	for i, path := range names {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatal(err)
		}
		relative[i] = filepath.ToSlash(rel)
	}
	sort.Strings(relative)
	for _, name := range relative {
		sum.Write([]byte(name + "\x00"))
		sum.Write(readFile(t, filepath.Join(dir, filepath.FromSlash(name))))
	}
	return hex.EncodeToString(sum.Sum(nil))[:12]
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// packAdapter builds the npm package this checkout would publish and returns
// the tarball's path.
//
// The scaffolded project installs that tarball rather than importing the
// checkout by a relative path, because a relative import is a configuration
// only this repository has: it would resolve files package.json's `files` list
// never mentions, and pass over exactly the mistake — a runtime file that never
// leaves for the registry — this test is the last chance to catch.
func packAdapter(t *testing.T, into string) string {
	t.Helper()
	if _, err := exec.LookPath("pnpm"); err != nil {
		t.Fatalf("pnpm is not on PATH, so this test cannot run — which is not the same as passing: %v", err)
	}
	source := filepath.Join(checkoutRoot(t), "internal", "adapter")
	cmd := exec.Command("pnpm", "pack", "--pack-destination", into)
	cmd.Dir = source
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("packing %s: %v\n%s", source, err, out)
	}
	found, err := filepath.Glob(filepath.Join(into, "*.tgz"))
	if err != nil || len(found) != 1 {
		t.Fatalf("packing %s produced %v (%v); want one tarball", source, found, err)
	}
	return found[0]
}

// packageVersion is the version the adapter package publishes at, read from
// package.json — the value the adapter stamps into every manifest it writes.
func packageVersion(t *testing.T, dir string) string {
	t.Helper()
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(readFile(t, filepath.Join(dir, "package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version == "" {
		t.Fatalf("%s/package.json declares no version", dir)
	}
	return pkg.Version
}

// manifestStamp is the identity the adapter wrote into the build.
func manifestStamp(t *testing.T, dir string) struct {
	Skgo    string `json:"skgo"`
	Adapter string `json:"skgoAdapter"`
} {
	t.Helper()
	var stamp struct {
		Skgo    string `json:"skgo"`
		Adapter string `json:"skgoAdapter"`
	}
	raw := readFile(t, filepath.Join(dir, "web", "build", "skgo.manifest.json"))
	if err := json.Unmarshal(raw, &stamp); err != nil {
		t.Fatal(err)
	}
	return stamp
}
