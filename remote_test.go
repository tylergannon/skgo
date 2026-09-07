package skgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/kithash"
	"github.com/tylergannon/skgo/internal/remotearg"
)

const testModule = "src/lib/todos.remote.ts"

type todo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func testRemotes(t *testing.T, cfg RemoteConfig, fns ...*Remote) *Remotes {
	t.Helper()
	rs, err := NewRemotes(cfg, fns...)
	if err != nil {
		t.Fatalf("NewRemotes: %v", err)
	}
	return rs
}

// envelope decodes the JSON wrapper and the devalue payload inside it.
func envelope(t *testing.T, body []byte) (kind string, data any, httpErr map[string]any) {
	t.Helper()
	var resp struct {
		Type  string         `json:"type"`
		Data  string         `json:"data"`
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decoding envelope %q: %v", body, err)
	}
	if resp.Type == "error" {
		return resp.Type, nil, resp.Error
	}
	parsed, err := devalue.Parse(resp.Data, nil)
	if err != nil {
		t.Fatalf("parsing devalue payload %q: %v", resp.Data, err)
	}
	return resp.Type, parsed, nil
}

// devalue.Parse hands objects back as *devalue.Object, so payload assertions
// go through these two helpers rather than a map type assertion.
func object(t *testing.T, v any) *devalue.Object {
	t.Helper()
	o, ok := v.(*devalue.Object)
	if !ok {
		t.Fatalf("value is %T, want an object", v)
	}
	return o
}

func field(t *testing.T, v any, key string) any {
	t.Helper()
	o := object(t, v)
	got, present := o.Get(key)
	if !present {
		t.Fatalf("object has no %q; keys are %v", key, o.Keys())
	}
	return got
}

func has(t *testing.T, v any, key string) bool {
	t.Helper()
	_, present := object(t, v).Get(key)
	return present
}

// node returns the `q` entry the client would read a query's value out of.
func node(t *testing.T, data any, key string) any {
	t.Helper()
	return field(t, field(t, data, "q"), key)
}

func TestQueryWithoutArgument(t *testing.T) {
	getTodos := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) ([]todo, error) {
		return []todo{{ID: "t1", Text: "write the adapter"}}, nil
	})
	rs := testRemotes(t, RemoteConfig{Version: "v1"}, getTodos)

	req := httptest.NewRequest(http.MethodGet, rs.Prefix()+getTodos.ID(), nil)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("cache-control = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
	if got := rec.Header().Get("X-Sveltekit-Version"); got != "v1" {
		t.Errorf("x-sveltekit-version = %q, want v1", got)
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}

	// With no argument the payload is absent, so the cache key ends in an
	// empty payload segment.
	n := node(t, data, getTodos.ID()+"/")
	list, ok := field(t, n, "v").([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("q node value = %#v, want a one-element list", field(t, n, "v"))
	}
	if got := field(t, list[0], "id"); got != "t1" {
		t.Errorf("first todo id = %#v", got)
	}
	if got := field(t, list[0], "text"); got != "write the adapter" {
		t.Errorf("first todo text = %#v", got)
	}

	// `_` carries the same value, as kit's does.
	if !has(t, data, "_") {
		t.Errorf("payload has no `_` field; keys are %v", object(t, data).Keys())
	}
	if has(t, data, "r") {
		t.Error("a plain query must not set r")
	}
}

func TestQueryWithArgument(t *testing.T) {
	getTodo := NewQuery(testModule, "getTodo", func(ctx context.Context, id string) (todo, error) {
		if id != "t2" {
			return todo{}, Errorf(404, "No todo with id %q", id)
		}
		return todo{ID: "t2", Text: "serve remote functions"}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, getTodo)

	payload, err := remotearg.StringifyQueryArg("t2")
	if err != nil {
		t.Fatalf("StringifyQueryArg: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, rs.Prefix()+getTodo.ID()+"?payload="+payload, nil)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}

	n := node(t, data, getTodo.ID()+"/"+payload)
	if got := field(t, field(t, n, "v"), "id"); got != "t2" {
		t.Fatalf("q node value id = %#v", got)
	}
}

func TestQueryErrorEnvelope(t *testing.T) {
	getTodo := NewQuery(testModule, "getTodo", func(ctx context.Context, id string) (todo, error) {
		return todo{}, Errorf(404, "No todo with id %q", id)
	})
	rs := testRemotes(t, RemoteConfig{}, getTodo)

	payload, _ := remotearg.StringifyQueryArg("nope")
	req := httptest.NewRequest(http.MethodGet, rs.Prefix()+getTodo.ID()+"?payload="+payload, nil)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	// Runtime remote errors travel with HTTP 200; the envelope carries the
	// status.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	kind, _, httpErr := envelope(t, rec.Body.Bytes())
	if kind != "error" {
		t.Fatalf("type = %q, want error", kind)
	}
	if httpErr["status"] != float64(404) {
		t.Errorf("error status = %v, want 404", httpErr["status"])
	}
	if !strings.Contains(httpErr["message"].(string), "nope") {
		t.Errorf("error message = %v", httpErr["message"])
	}
}

func TestUnknownHashAndNameAre404(t *testing.T) {
	known := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })
	rs := testRemotes(t, RemoteConfig{}, known)

	for _, path := range []string{
		rs.Prefix() + "nosuch/getTodos",
		rs.Prefix() + kithash.Kit(testModule) + "/nosuch",
		rs.Prefix() + "onlyone",
	} {
		rec := httptest.NewRecorder()
		rs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		kind, _, httpErr := envelope(t, rec.Body.Bytes())
		if kind != "error" {
			t.Fatalf("%s: type = %q, want error", path, kind)
		}
		if httpErr["status"] != float64(404) || httpErr["message"] != "Error: 404" {
			t.Errorf("%s: error = %#v, want {404, \"Error: 404\"}", path, httpErr)
		}
	}
}

func TestCrossOriginCommandIsForbidden(t *testing.T) {
	add := NewCommand(testModule, "addTodo", func(ctx context.Context, text string) (todo, error) {
		return todo{ID: "t3", Text: text}, nil
	})
	rs := testRemotes(t, RemoteConfig{Origin: "http://127.0.0.1:8080"}, add)

	body := strings.NewReader(`{"payload":"","refreshes":[]}`)
	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+add.ID(), body)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var refusal map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil {
		t.Fatalf("decoding refusal %q: %v", rec.Body.String(), err)
	}
	if refusal["message"] != "Cross-site remote requests are forbidden" {
		t.Errorf("refusal = %#v", refusal)
	}

	// The matching origin is allowed through.
	req = httptest.NewRequest(http.MethodPost, rs.Prefix()+add.ID(), strings.NewReader(`{"payload":"","refreshes":[]}`))
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rec = httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("same-origin status = %d, want 200", rec.Code)
	}

	// A GET is never cross-site-forbidden, whatever its Origin.
	q := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })
	rs = testRemotes(t, RemoteConfig{Origin: "http://127.0.0.1:8080"}, q)
	req = httptest.NewRequest(http.MethodGet, rs.Prefix()+q.ID(), nil)
	req.Header.Set("Origin", "http://evil.example")
	rec = httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cross-origin GET status = %d, want 200", rec.Code)
	}
}

func TestCommandRefreshes(t *testing.T) {
	var texts []string
	getTodosFn := func(ctx context.Context) ([]string, error) {
		out := make([]string, len(texts))
		copy(out, texts)
		return out, nil
	}
	getTodos := NewQueryNoArg(testModule, "getTodos", getTodosFn)
	addTodo := NewCommand(testModule, "addTodo", func(ctx context.Context, text string) (todo, error) {
		texts = append(texts, text)
		return todo{ID: "t3", Text: text}, RefreshRequestedNoArg(ctx, getTodosFn)
	})
	rs := testRemotes(t, RemoteConfig{}, getTodos, addTodo)

	payload, err := remotearg.StringifyCommandArg("buy milk")
	if err != nil {
		t.Fatalf("StringifyCommandArg: %v", err)
	}
	refreshKey := getTodos.ID() + "/"
	body, _ := json.Marshal(map[string]any{
		"payload": payload,
		"refreshes": []string{
			refreshKey,
			"nosuchhash/getTodos/", // unknown id: skipped in silence
			addTodo.ID() + "/",     // a command is not refreshable: skipped
			"nothing-that-parses",  // no slash at all: skipped
		},
	})

	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+addTodo.ID(), strings.NewReader(string(body)))
	req.Header.Set("X-Sveltekit-Pathname", "/")
	req.Header.Set("X-Sveltekit-Search", "")
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}

	// A command's own result is read out of `_`.
	if got := field(t, field(t, data, "_"), "text"); got != "buy milk" {
		t.Errorf("`_`.text = %#v", got)
	}
	if got := field(t, data, "r"); got != true {
		t.Errorf("r = %#v, want true", got)
	}

	q := object(t, field(t, data, "q"))
	if q.Len() != 1 {
		t.Fatalf("q has %d entries (%v), want only the known refresh", q.Len(), q.Keys())
	}
	list, _ := field(t, node(t, data, refreshKey), "v").([]any)
	if len(list) != 1 || list[0] != "buy milk" {
		t.Errorf("refreshed value = %#v", list)
	}
}

func TestCommandWithoutRefreshesOmitsQAndR(t *testing.T) {
	addTodo := NewCommand(testModule, "addTodo", func(ctx context.Context, text string) (todo, error) {
		return todo{ID: "t3", Text: text}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, addTodo)

	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+addTodo.ID(), strings.NewReader(`{"payload":""}`))
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	_, data, _ := envelope(t, rec.Body.Bytes())
	if has(t, data, "q") {
		t.Error("q present with no refreshes")
	}
	if has(t, data, "r") {
		t.Error("r present with no refreshes")
	}
}

func TestInterceptOnlyClaimsThePrefix(t *testing.T) {
	q := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 7, nil })
	rs := testRemotes(t, RemoteConfig{}, q)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next", "yes")
		w.WriteHeader(http.StatusTeapot)
	})
	h := rs.Intercept(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("non-remote request was not passed through: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, rs.Prefix()+q.ID(), nil))
	if rec.Header().Get("X-Next") != "" {
		t.Errorf("remote request fell through to the next handler")
	}
}

func TestPrefixHonoursBaseAndAppDir(t *testing.T) {
	rs := testRemotes(t, RemoteConfig{Base: "/app", AppDir: "_kit"})
	if got := rs.Prefix(); got != "/app/_kit/remote/" {
		t.Errorf("Prefix() = %q", got)
	}
	rs = testRemotes(t, RemoteConfig{})
	if got := rs.Prefix(); got != "/_app/remote/" {
		t.Errorf("Prefix() = %q", got)
	}
}

func TestReadManifestAndRemoteConfig(t *testing.T) {
	m, err := ReadManifest(buildFrom("devel", thisAdapter(t)))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	cfg := m.RemoteConfig("http://127.0.0.1:8080")
	if cfg.AppDir != "_app" || cfg.Version != "1737000000000" || cfg.Origin != "http://127.0.0.1:8080" {
		t.Errorf("RemoteConfig = %#v", cfg)
	}
}

func TestDuplicateRegistrationIsAnError(t *testing.T) {
	a := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })
	b := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 2, nil })
	if _, err := NewRemotes(RemoteConfig{}, a, b); err == nil {
		t.Fatal("registering the same id twice should fail")
	}
}

func TestMethodMismatchIs405(t *testing.T) {
	q := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })
	c := NewCommandNoArg(testModule, "addTodo", func(ctx context.Context) (int, error) { return 1, nil })
	rs := testRemotes(t, RemoteConfig{}, q, c)

	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, rs.Prefix()+q.ID(), strings.NewReader("{}")))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to a query = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	rs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, rs.Prefix()+c.ID(), nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET to a command = %d, want 405", rec.Code)
	}
}
