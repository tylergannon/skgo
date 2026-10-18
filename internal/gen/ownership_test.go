package gen

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// Projecting a named type means loading the Go package that declares it and
// handing polytype the type. For a type the app declares, that package is the
// app's own. For a type from a dependency it is the module cache: source the
// app does not own and, for any real consumer, cannot write.
//
// These tests hold the line at the filesystem. A named type from another module
// has to reach the browser, nothing may appear outside the app's module while
// it does, and nothing but the TypeScript may appear inside it either: the
// declaration is polytype's to emit, and it emits it without leaving a marker,
// a schema or a Go file behind.

// foreignFixture lays out three modules in a temp dir: the app, a library it
// depends on, and a stand-in for skgo at skgo's own import path. Markers are
// recognised by import path, so the stand-in is what the scanner sees in a
// downstream project, without dragging the real module (or its dependencies,
// or a network) into the test.
//
// remote is the body of `<app>/web/src/data/data.remote.go`; extra adds any
// further files, keyed by their slash-separated path under the fixture root.
func foreignFixture(t *testing.T, remote string, extra map[string]string) (root string, cfg Config) {
	t.Helper()
	return foreignFixtureIn(t, t.TempDir(), remote, extra)
}

func foreignFixtureIn(t *testing.T, root string, remote string, extra map[string]string) (string, Config) {
	t.Helper()
	// The fixture is not a member of this repository's workspace, so the go
	// commands the generator runs have to see the fixture's own go.mod.
	// GOWORK is off for the whole package (TestMain).

	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("skgo/go.mod", "module github.com/tylergannon/skgo\n\ngo 1.27.1\n")
	write("skgo/skgo.go", `package skgo

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
)

type Marker struct{}

type PrerenderOptions struct {
	Inputs   any
	Dynamic  bool
	Validate any
}

type Transporter struct {
	Type   reflect.Type
	Encode func(any) (any, error)
	Decode func(any) (any, error)
}

type Transport map[string]Transporter

func Query(fn any) Marker { _ = fn; return Marker{} }

type Event struct{}

func (*Event) Request() *http.Request         { return &http.Request{} }
func RequestLocals[L any](context.Context) *L { return new(L) }

type RequestResolve[P, L any] func(context.Context, RequestEvent[P, L], ...ResolveOptions) (*http.Response, error)
type RequestMiddleware[P, L any] func(context.Context, RequestEvent[P, L], RequestResolve[P, L]) (*http.Response, error)
type RequestHandle[P, L any] func(context.Context, RequestEvent[P, L]) (RequestEvent[P, L], error)
type ResolveOptions struct{}
type Fetch func(*http.Request) (*http.Response, error)
type RerouteEvent struct {
	URL   *url.URL
	Fetch Fetch
}
type Reroute func(context.Context, RerouteEvent) (*string, error)
type HandleFetch func(context.Context, *http.Request, Fetch) (*http.Response, error)
type CaughtError struct {
	Kind    string
	Status  int
	Message string
	Err     error
}
type HandleError func(context.Context, CaughtError) map[string]any
type ApplicationConfig struct {
	HandleFetch                     HandleFetch
	HandleError                     HandleError
	FilterSerializedResponseHeaders func(name, value string) bool
}
type HandleConfig struct {
	Matchers      map[string]ParamMatcher
	ClientAddress func(*http.Request) (string, error)
	HandleError   HandleError
}

func HookValues(*Event) (string, map[string]any)                                   { return "", nil }
func Errorf(int, string, ...any) error                                             { return nil }
func (h RequestHandle[P, L]) Middleware() RequestMiddleware[P, L]                  { return nil }
func RequestSequence[P, L any](...RequestMiddleware[P, L]) RequestMiddleware[P, L] { return nil }
func (h RequestMiddleware[P, L]) Intercept(HandleConfig, func(*Event) (P, error), http.Handler) http.Handler {
	return nil
}

type RequestEvent[P, L any] struct {
	*Event
	Params P
	Locals *L
}

func (RequestEvent[P, L]) Context() context.Context { return context.Background() }

type ParamMatcher func(string) (any, bool)
type CallerMatchers map[string]ParamMatcher
type ManifestParam struct {
	Name     string
	Optional bool
	Rest     bool
	Chained  bool
	Matcher  string
}
type CallerRoute struct {
	Params    []ManifestParam
	NewParams func(map[string]any) (any, error)
}
type CallerRoutes map[string]CallerRoute

func RemoteCallerValues(*Event) (string, map[string]any) { return "", nil }
func EventFrom(context.Context) *Event                   { return &Event{} }
func TrackLoadParam(*Event, string)                      {}
func LoadParamValue[T any](*Event, string) T             { var zero T; return zero }
func OptionalLoadParamValue[T any](*Event, string) *T    { return nil }

type RouteEntries struct{}

func Entries[P any](func() ([]P, error)) Marker                           { return Marker{} }
func NewRouteEntries[P any](string, func() ([]P, error)) *RouteEntries    { return nil }
func Load[P, L, Out any](fn func(RequestEvent[P, L]) (Out, error)) Marker { _ = fn; return Marker{} }

func Command(fn any) Marker { _ = fn; return Marker{} }

func LiveQuery(fn any) Marker { _ = fn; return Marker{} }

func BatchQuery(fn any) Marker { _ = fn; return Marker{} }

func Prerender(fn any, options ...PrerenderOptions) Marker { _, _ = fn, options; return Marker{} }

func Form(fn any) Marker { _ = fn; return Marker{} }

type Kind int

const (
	KindQuery Kind = iota
	KindCommand
	KindLive
	KindBatch
	KindForm
	KindPrerender
)

type Deferred[T any] struct{}

func Transported[T any](tag string) Marker { _ = tag; return Marker{} }

func ActionNoData(fn func(context.Context) error) Marker { _ = fn; return Marker{} }

func Action[Out any](fn func(context.Context) (Out, error)) Marker { _ = fn; return Marker{} }

func ActionWithFailure[Out, Failure any](fn func(context.Context) (Out, error), failure Failure) Marker {
	_, _ = fn, failure
	return Marker{}
}

func DefaultAction[Out any](fn func(context.Context) (Out, error)) Marker { _ = fn; return Marker{} }

func GET(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func POST(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func PUT(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func PATCH(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func DELETE(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func OPTIONS(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func HEAD(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func QUERY(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }

func Fallback(fn http.HandlerFunc) Marker { _ = fn; return Marker{} }
`)

	// `wire` is the dependency. Its types are the ones that have to travel: a
	// struct, and an enum reached only through that struct's field.
	write("wire/go.mod", "module example.com/wire\n\ngo 1.27.1\n")
	write("wire/wire.go", `package wire

type Thing struct {
	Name   string `+"`json:\"name\"`"+`
	Status Status `+"`json:\"status\"`"+`
}

type Status string

func (Status) enum() {}

const (
	StatusOn  Status = "on"
	StatusOff Status = "off"
)
`)

	// A second dependency package, also called `wire` and also declaring a
	// `Thing`. Two libraries picking the same short name is an ordinary thing
	// to have, and neither name is the app's to change.
	write("wire/other/wire/wire.go", "package wire\n\ntype Thing struct {\n\tLabel string `json:\"label\"`\n}\n")

	goMod, sum := fixtureModule(t)
	write("app/go.mod", goMod)
	write("app/go.sum", sum)
	if err := os.MkdirAll(filepath.Join(root, "app", "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range extra {
		write(path, content)
	}
	if remote != "" {
		write("app/web/src/data/data.remote.go", remote)
	}
	kitRoot, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(kitRoot, "example", "web", "node_modules", "@sveltejs", "kit")
	if _, err := os.Stat(filepath.Join(kit, "src", "utils", "routing.js")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "app", "web", "node_modules", "@sveltejs", "kit")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(kit, link); err != nil {
		t.Fatal(err)
	}

	return root, fixtureConfig(Config{
		Web:  filepath.Join(root, "app", "web"),
		Out:  filepath.Join(root, "app", "generated"),
		Logf: func(string, ...any) {},
	})
}

// fixtureModule builds the fixture app's go.mod out of the example app's, so
// the fixture resolves the polytype this repository actually pins rather than
// a second copy of the version number.
func fixtureModule(t *testing.T) (goMod, goSum string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "example", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := modfile.Parse("go.mod", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("module example.com/app\n\n")
	fmt.Fprintf(&b, "go %s\n\nrequire (\n", parsed.Go.Version)
	b.WriteString("\texample.com/wire v0.0.0\n")
	for _, req := range parsed.Require {
		if req.Mod.Path == skgoPkg {
			continue
		}
		fmt.Fprintf(&b, "\t%s %s\n", req.Mod.Path, req.Mod.Version)
	}
	fmt.Fprintf(&b, "\t%s v0.0.0\n)\n\n", skgoPkg)
	b.WriteString("replace example.com/wire => ../wire\n\n")
	fmt.Fprintf(&b, "replace %s => ../skgo\n", skgoPkg)

	sum, err := os.ReadFile(filepath.Join("..", "..", "example", "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	return b.String(), string(sum)
}

// filesUnder lists every regular file below dir, relative to it, sorted.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
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

func readFixtureFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(raw)
}
