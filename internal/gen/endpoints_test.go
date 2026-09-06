package gen

import (
	"strings"
	"testing"
)

// endpointSource is a `server.go` declaring the two methods a route answers.
const endpointSource = `package thing

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func read(w http.ResponseWriter, r *http.Request)  { _, _ = w.Write([]byte("read")) }
func write(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("write")) }

var (
	_ = skgo.GET(read)
	_ = skgo.POST(write)
)
`

// A `server.go` becomes the `+server.ts` kit compiles, with one export per
// marker, named exactly as kit names them — kit dispatches by looking the
// request's method up on the module's exports, so the names are the interface.
func TestAServerRouteBecomesTheModuleKitCompiles(t *testing.T) {
	root, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/api/thing/server.go": endpointSource,
	})

	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	stub := readFixtureFile(t, root, "app/web/src/routes/api/thing/+server.ts")
	for _, want := range []string{"export const GET =", "export const POST ="} {
		if !strings.Contains(stub, want) {
			t.Errorf("the generated +server.ts has no %q:\n%s", want, stub)
		}
	}
	// Every body throws, so a real response proves Go answered rather than this
	// module — the same rule the remote and load stubs follow.
	if !strings.Contains(stub, "throw new Error('skgo: implemented in Go')") {
		t.Errorf("the generated +server.ts does not throw:\n%s", stub)
	}

	bindings := readFixtureFile(t, root, "app/web/src/routes/api/thing/skgo_remotes_gen.go")
	for _, want := range []string{
		`skgo.NewEndpoint("/api/thing", "GET", read)`,
		`skgo.NewEndpoint("/api/thing", "POST", write)`,
	} {
		if !strings.Contains(bindings, want) {
			t.Errorf("the registration file has no %q:\n%s", want, bindings)
		}
	}

	list := readFixtureFile(t, root, "app/web/skgo.remotes.json")
	if !strings.Contains(list, `"/api/thing"`) || !strings.Contains(list, `"GET"`) {
		t.Errorf("skgo.remotes.json does not describe the route:\n%s", list)
	}
}

// Kit's route id is the route directory's path below `src/routes`, verbatim:
// group segments and parameter segments are part of it, because kit reads the
// directory back with `path.join(cwd, routes_base, id)`.
func TestARouteIdIsTheDirectoryPathVerbatim(t *testing.T) {
	cases := map[string]string{
		"src/routes/+server.ts":                      "/",
		"src/routes/api/thing/+server.ts":            "/api/thing",
		"src/routes/(marketing)/plans/+server.ts":    "/(marketing)/plans",
		"src/routes/items/[id]/+server.ts":           "/items/[id]",
		"src/routes/docs/[...rest]/edit/+server.ts":  "/docs/[...rest]/edit",
		"src/routes/[[lang]]/about/x/y/z/+server.ts": "/[[lang]]/about/x/y/z",
	}
	for module, want := range cases {
		got, err := routeIDFor(module)
		if err != nil {
			t.Errorf("routeIDFor(%q): %v", module, err)
			continue
		}
		if got != want {
			t.Errorf("routeIDFor(%q) = %q, want %q", module, got, want)
		}
	}

	if _, err := routeIDFor("src/lib/+server.ts"); err == nil {
		t.Error("a +server.ts outside src/routes was accepted")
	}
}

// The markers are per file, because the file name is what decides which module
// kit compiles beside it. A marker in the wrong file is a stub nobody generates.
func TestAServerRouteMarkerBelongsInServerGo(t *testing.T) {
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/api/thing/thing.remote.go": endpointSource,
	})

	err := Run(cfg)
	if err == nil {
		t.Fatal("a server route declared in a .remote.go was accepted")
	}
	if !strings.Contains(err.Error(), "declares a server route") {
		t.Errorf("the error does not say where a server route belongs: %v", err)
	}
}

// A module has one export per name, so a route cannot answer one method twice.
func TestARouteCannotAnswerAMethodTwice(t *testing.T) {
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/api/thing/server.go": `package thing

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func read(w http.ResponseWriter, r *http.Request)  { _, _ = w.Write([]byte("read")) }
func again(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("again")) }

var (
	_ = skgo.GET(read)
	_ = skgo.GET(again)
)
`,
	})

	err := Run(cfg)
	if err == nil {
		t.Fatal("two GET handlers for one route were accepted")
	}
	if !strings.Contains(err.Error(), "answers GET twice") {
		t.Errorf("the error does not name the method: %v", err)
	}
}
