package skgo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/ssr"
)

// Positive receipts count literal refusals, rather than merely looking for an
// absent value (which a callback that never executed would satisfy).
func restrictedCallerReceipt(ctx context.Context) (string, error) {
	e := EventFrom(ctx)
	if e == nil {
		return "missing event", nil
	}
	refused := 0
	for _, read := range []func(){func() { e.Param("id") }, func() { e.Params() }, func() { e.URL() }, func() { e.RouteID() }, func() { e.SearchParam("secret") }, func() { RemoteCallerValues(e) }} {
		func() {
			defer func() {
				if recover() != nil {
					refused++
				}
			}()
			read()
		}()
	}
	if e.SetCookie("query", "no", CookieOptions{}) == nil {
		return "query wrote cookie", nil
	}
	return fmt.Sprintf("query:refused:%d", refused), nil
}

func TestRemoteCallerQueryRestrictionsAtServedEntries(t *testing.T) {
	q := NewQueryNoArg(testModule, "restrictedQuery", restrictedCallerReceipt)
	b := NewBatchQuery(testModule, "restrictedBatch", func(ctx context.Context, in []string) ([]string, error) {
		got, err := restrictedCallerReceipt(ctx)
		return []string{got}, err
	})
	live := NewLiveQueryNoArg(testModule, "restrictedLive", func(ctx context.Context, yield func(string) error) error {
		got, err := restrictedCallerReceipt(ctx)
		if err != nil {
			return err
		}
		return yield(got)
	})
	p := NewRemote(RemoteSpec{Kind: KindPrerender, Module: testModule, Name: "restrictedPrerender", Call: func(ctx context.Context, _ Call) (any, error) { return restrictedCallerReceipt(ctx) }})
	rs := testRemotes(t, RemoteConfig{Dev: true}, q, b, live, p)
	for _, tc := range []struct{ name, method, path, body string }{
		{"query", "GET", rs.Prefix() + q.ID(), ""},
		{"batch", "POST", rs.Prefix() + b.ID(), `{"payloads":["WyJ4Il0"]}`},
		{"live", "GET", rs.Prefix() + live.ID(), ""},
		{"dev-prerender", "GET", rs.Prefix() + p.ID(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("x-sveltekit-pathname", "/numeric/42")
			req.Header.Set("x-sveltekit-search", "?secret=caller")
			// Put a usable command caller in the inherited context. The query
			// constructor must not retain it even though the cookie jar is shared.
			req = req.WithContext(callerContext(req.Context(), &remoteCaller{url: mustURL(t, "http://fixture/numeric/42"), routeID: "/numeric/[id=Numeric]", values: map[string]any{"id": 42}}))
			rec := httptest.NewRecorder()
			rs.ServeHTTP(rec, req)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "query:refused:6") {
				t.Fatalf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("query cookie escaped")
			}
		})
	}
}

func TestRemoteCallerRefreshedQueryRestrictions(t *testing.T) {
	qFn := restrictedCallerReceipt
	q := NewQueryNoArg(testModule, "restrictedRefresh", qFn)
	for _, requested := range []bool{false, true} {
		t.Run(fmt.Sprint(requested), func(t *testing.T) {
			cmd := NewCommandNoArg(testModule, "refreshCaller", func(ctx context.Context) (string, error) {
				e := EventFrom(ctx)
				if err := e.SetCookie("command", "yes", CookieOptions{}); err != nil {
					return "", err
				}
				if requested {
					return "command:ran", RefreshRequestedNoArg(ctx, qFn)
				}
				return "command:ran", RefreshNoArg(ctx, qFn)
			})
			rs := testRemotes(t, RemoteConfig{}, q, cmd)
			var refreshes []string
			if requested {
				refreshes = []string{q.ID() + "/"}
			}
			rec := postCommand(t, rs, cmd, devalue.Undefined, refreshes)
			_, data, _ := envelope(t, rec.Body.Bytes())
			if got := field(t, node(t, data, q.ID()+"/"), "v"); got != "query:refused:6" {
				t.Fatalf("refreshed restriction receipt %v", got)
			}
			if len(rec.Result().Cookies()) != 1 || rec.Result().Cookies()[0].Name != "command" {
				t.Fatalf("cookies: %v", rec.Result().Cookies())
			}
		})
	}
}

func TestRemoteCallerSSRQueryRestrictions(t *testing.T) {
	q := NewQueryNoArg(testModule, "restrictedSSR", restrictedCallerReceipt)
	rs := testRemotes(t, RemoteConfig{}, q)
	// Exercise renderPlan's actual host event construction and embedded engine.
	bundle := fmt.Sprintf(`globalThis.__skgo_ping=()=> 'ok';
 globalThis.__skgo_render=()=>{const result={done:false};__skgo_remote(%q,'').then(value=>{result.body=value;result.done=true},error=>{result.failure=String(error);result.done=true});return result;};`, q.ID())
	engine, err := ssr.New("restricted-caller.js", []byte(bundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	renderer := &SSR{remotes: rs, loads: &Loads{}, engine: engine}
	req := httptest.NewRequest("GET", "/numeric/42", nil)
	req = req.WithContext(callerContext(req.Context(), &remoteCaller{url: mustURL(t, "http://fixture/numeric/42"), routeID: "/numeric/[id=Numeric]", values: map[string]any{"id": 42}}))
	result, answers, err := renderer.renderPlan(req, dataRequest{url: mustURL(t, "http://fixture/numeric/42")}, documentPlan{routeID: "/numeric/[id=Numeric]", params: map[string]string{"id": "42"}, status: 200}, newDocumentCSPWithNonce(nil, "fixed"))
	if err != nil || !strings.Contains(result.Body, "query:refused:6") || answers["q"][q.ID()+"/"].tree != "query:refused:6" {
		t.Fatalf("SSR query receipt: %q %v %v", result.Body, answers, err)
	}
}

func TestRemoteCallerBuildPrerenderQueryRestrictions(t *testing.T) {
	p := NewRemote(RemoteSpec{Kind: KindPrerender, Module: testModule, Name: "restrictedBuild", Call: func(ctx context.Context, _ Call) (any, error) { return restrictedCallerReceipt(ctx) }})
	input := fmt.Sprintf(`{"module":%q,"name":"restrictedBuild","url":"http://fixture/numeric/42","params":{"id":"42"}}`, testModule)
	var output strings.Builder
	if err := callBuildOperation("/remote", strings.NewReader(input), &output, nil, nil, []*Remote{p}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "query:refused:6") {
		t.Fatalf("build query receipt: %s", output.String())
	}
}

func TestTypedCommandContextRetainsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/_app/remote/id/act", nil).WithContext(ctx)
	e := (&Remotes{}).newEvent(req, true)
	event := RequestEvent[struct{}, struct{}]{Event: e}
	derived := event.Context()
	cancel()
	if derived.Err() != context.Canceled {
		t.Fatal("typed event lost cancellation")
	}
}

func TestRemoteCallerURLKeepsEscapedPath(t *testing.T) {
	fn := NewCommandNoArg(testModule, "callerURL", func(ctx context.Context) (string, error) { return EventFrom(ctx).URL().EscapedPath(), nil })
	rs := testRemotes(t, RemoteConfig{}, fn)
	for _, path := range []string{"/text/%2525", "/text/a%2Fb", "/text/%E2%9C%93"} {
		req := httptest.NewRequest("POST", rs.Prefix()+fn.ID(), strings.NewReader(`{"payload":"","refreshes":[]}`))
		req.Header.Set("x-sveltekit-pathname", path)
		rec := httptest.NewRecorder()
		rs.ServeHTTP(rec, req)
		_, data, _ := envelope(t, rec.Body.Bytes())
		if got := field(t, data, "_"); got != path {
			t.Fatalf("caller URL want literal %q got %v", path, got)
		}
	}
}
