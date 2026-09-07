package skgo

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/ssr"
)

// The strings this file compares against were written out of kit's own source
// (`packages/kit/src/runtime/server/page/render.js` and `data_serializer.js` at
// @sveltejs/kit 3.0.0-next.25), not read back off skgo's output. That is the
// point of them: a document skgo assembles differently from kit's is a document
// kit's client is not the client for.

// streamer is a renderer with everything the document assembly reads and
// nothing else. The engine is not one of those things — `assemble` is the half
// of `render_response` that executes no code — so a test of the document does
// not need a build, a bundle or a runtime.
func streamer(transport Transport) *SSR {
	return &SSR{
		loads: &Loads{cfg: LoadConfig{Transport: transport}},
		info: ManifestSSR{
			GlobalName: "__sveltekit_test",
			Client: ManifestClient{
				Start: "_app/immutable/entry/start.js",
				App:   "_app/immutable/entry/app.js",
			},
			Nodes: []ManifestSSRNode{{Index: 0}, {Index: 5}},
		},
		template: "<!doctype html>\n<html>\n\t<head>\n\t\t%sveltekit.head%\n\t</head>\n\t<body>\n\t\t<div>%sveltekit.body%</div>\n\t</body>\n</html>\n",
		version:  "1234",
	}
}

// promising is the shape a load with a deferred field produces once the branch
// has been encoded: an ordinary tree with the Deferred left in it.
func promising(fields map[string]any) dataNode {
	return dataNode{kind: "data", data: fields}
}

func pending() *deferred { return &deferred{done: make(chan struct{})} }

func fulfil(d *deferred, value any) {
	d.value = value
	close(d.done)
}

func plannedPage(nodes ...dataNode) documentPlan {
	return documentPlan{
		routeID: "/account/orders",
		params:  map[string]string{},
		status:  http.StatusOK,
		hydrate: true,
		indices: []int{0, 1},
		nodes:   nodes,
		errors:  []*int{nil, nil},
	}
}

func rendered() ssr.Result {
	return ssr.Result{Done: true, Status: http.StatusOK, Body: "<p>fetching the orders…</p>"}
}

func assembled(t *testing.T, s *SSR, plan documentPlan) (string, *promiseTable) {
	t.Helper()
	req := dataRequest{url: mustURL(t, "http://127.0.0.1/account/orders")}
	document, promises, err := s.assemble(req, plan, rendered(), nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return document, promises
}

func TestDocumentCarriesTheDeferredPlaceholderAndKitsPlumbing(t *testing.T) {
	orders := pending()
	s := streamer(nil)
	document, promises := assembled(t, s, plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"orderTotal": float64(2), "orders": orders}),
	))

	if len(promises.order) != 1 || promises.ids[orders] != 1 {
		t.Fatalf("promise ids: %v", promises.ids)
	}

	// Kit's hydration array holds `<global>.defer(<id>)` where the promise was,
	// and the value the load did have beside it.
	const hydrated = `data: [{type:"data",data:{who:"ada"},uses:{}},{type:"data",data:{orderTotal:2,orders:__sveltekit_test.defer(1)},uses:{}}]`
	if !strings.Contains(document, hydrated) {
		t.Errorf("the hydration array does not carry the placeholder:\n%s", document)
	}

	// The map the two halves of each promise are kept in is declared before the
	// global, because the global's own properties close over it.
	deferredMap := strings.Index(document, "const deferred = new Map();")
	global := strings.Index(document, "__sveltekit_test = {")
	if deferredMap < 0 || global < 0 || deferredMap > global {
		t.Errorf("`const deferred = new Map();` is not declared before the global (%d, %d)", deferredMap, global)
	}

	for _, want := range []string{
		"defer: (id) => new Promise((fulfil, reject) => {\n" +
			"\t\t\t\t\t\t\tdeferred.set(id, { fulfil, reject });\n" +
			"\t\t\t\t\t\t})",
		"resolve: async (id, fn) => {\n" +
			"\t\t\t\t\t\t\tconst [data, error] = fn();\n" +
			"\n" +
			"\t\t\t\t\t\t\tconst try_to_resolve = () => {\n" +
			"\t\t\t\t\t\t\t\tif (!deferred.has(id)) {\n" +
			"\t\t\t\t\t\t\t\t\tsetTimeout(try_to_resolve, 0);\n" +
			"\t\t\t\t\t\t\t\t\treturn;\n" +
			"\t\t\t\t\t\t\t\t}\n" +
			"\t\t\t\t\t\t\t\tconst { fulfil, reject } = deferred.get(id);\n" +
			"\t\t\t\t\t\t\t\tdeferred.delete(id);\n" +
			"\t\t\t\t\t\t\t\tif (error) reject(error);\n" +
			"\t\t\t\t\t\t\t\telse fulfil(data);\n" +
			"\t\t\t\t\t\t\t}\n" +
			"\t\t\t\t\t\t\ttry_to_resolve();\n" +
			"\t\t\t\t\t\t}",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("the boot script is missing kit's own:\n%s\n\ngot:\n%s", want, document)
		}
	}
}

// A page with nothing outstanding must not grow kit's promise plumbing: the
// `if (chunks)` branch of render.js is the only place it comes from.
func TestADocumentWithNothingOutstandingCarriesNoPlumbing(t *testing.T) {
	s := streamer(nil)
	document, promises := assembled(t, s, plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"orderTotal": float64(2)}),
	))
	if len(promises.order) != 0 {
		t.Fatalf("promises: %d", len(promises.order))
	}
	for _, unwanted := range []string{"const deferred", "defer:", "resolve:", ".defer("} {
		if strings.Contains(document, unwanted) {
			t.Errorf("a document with nothing outstanding carries %q:\n%s", unwanted, document)
		}
	}
}

// With a transport hook the chunk's expression calls `app.decode`, so the arrow
// takes `app` and the boot script imports the client's app module before it
// evaluates the chunk. Kit switches on `has_custom_transporters` for the second
// and on `str.includes('app.decode')` for the first.
func TestATransportedValueStreamsThroughTheAppsDecoder(t *testing.T) {
	type cents int
	transport := Transport{"Money": {
		Type:   reflect.TypeOf(cents(0)),
		Encode: func(v any) (any, error) { return []any{float64(v.(cents))}, nil },
		Decode: func(any) (any, error) { return nil, nil },
	}}

	price := pending()
	s := streamer(transport)
	_, promises := assembled(t, s, plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"price": price}),
	))

	chunk := s.chunkScript(promises.ids[price], cents(4500), nil, s.deferReplacer(promises))
	const want = `<script>__sveltekit_test.resolve(1, (app) => [app.decode("Money", [4500])])</script>` + "\n"
	if chunk != want {
		t.Errorf("chunk\n got %q\nwant %q", chunk, want)
	}
}

// The pair a chunk carries is kit's `[data]` / `[, error]`: a hole in the first
// slot is what makes the boot script's `const [data, error]` reject.
func TestADeferredValueThatFailedStreamsAsAHoleAndAnError(t *testing.T) {
	failed := pending()
	s := streamer(nil)
	_, promises := assembled(t, s, plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"orders": failed}),
	))

	chunk := s.chunkScript(promises.ids[failed], nil, Errorf(402, "Your account is in arrears"), s.deferReplacer(promises))
	const want = `<script>__sveltekit_test.resolve(1, () => [,{status:402,message:"Your account is in arrears"}])</script>` + "\n"
	if chunk != want {
		t.Errorf("chunk\n got %q\nwant %q", chunk, want)
	}
}

// The whole of kit's streaming branch, in one response:
//
//	new Response(stream_text(transformed + '\n', chunks), { headers })
//
// — the document, a newline, the chunks — with the two quirks that fall out of
// how kit builds it. `headers` never had the etag put on it (`if (!chunks)`),
// and `status` is passed to `text()` on the other branch and to nothing here,
// so a page that earned a 418 is answered 200.
func TestAStreamedDocumentCarriesNeitherTheStatusNorAnEtag(t *testing.T) {
	orders := pending()
	s := streamer(nil)
	plan := plannedPage(
		promising(map[string]any{"who": "ada"}),
		promising(map[string]any{"orderTotal": float64(2), "orders": orders}),
	)
	plan.status = http.StatusTeapot
	document, promises := assembled(t, s, plan)

	go func() {
		time.Sleep(20 * time.Millisecond)
		fulfil(orders, []map[string]any{{"item": "a slow parcel"}})
	}()

	rec := httptest.NewRecorder()
	s.stream(rec, httptest.NewRequest(http.MethodGet, "/account/orders", nil), nil, document, promises)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200 — kit's streaming branch passes none", rec.Code)
	}
	if etag := rec.Header().Get("ETag"); etag != "" {
		t.Errorf("ETag: got %q, want none — kit sets it only `if (!chunks)`", etag)
	}
	if length := rec.Header().Get("Content-Length"); length != "" {
		t.Errorf("Content-Length: got %q, want none", length)
	}
	if page := rec.Header().Get("X-Sveltekit-Page"); page != "true" {
		t.Errorf("X-Sveltekit-Page: got %q", page)
	}

	want := document + "\n" +
		`<script>__sveltekit_test.resolve(1, () => [[{item:"a slow parcel"}]])</script>` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body\n got %q\nwant %q", rec.Body.String(), want)
	}
}

// Kit sends a chunk the moment its promise settles, not in the order the ids
// were handed out: `create_async_iterator` gives a settling promise the next
// free slot rather than its own. A page whose second promise is quicker sees it
// first, and the id in each chunk is what tells the client which is which.
func TestChunksAreSentInTheOrderTheySettle(t *testing.T) {
	slow, quick := pending(), pending()
	s := streamer(nil)
	document, promises := assembled(t, s, plannedPage(
		promising(map[string]any{"slow": slow}),
		promising(map[string]any{"quick": quick}),
	))
	if promises.ids[slow] != 1 || promises.ids[quick] != 2 {
		t.Fatalf("ids: slow %d, quick %d", promises.ids[slow], promises.ids[quick])
	}

	go func() {
		fulfil(quick, "second in, first out")
		time.Sleep(50 * time.Millisecond)
		fulfil(slow, "first in, last out")
	}()

	rec := httptest.NewRecorder()
	s.stream(rec, httptest.NewRequest(http.MethodGet, "/account/orders", nil), nil, document, promises)

	body := strings.TrimPrefix(rec.Body.String(), document+"\n")
	want := `<script>__sveltekit_test.resolve(2, () => ["second in, first out"])</script>` + "\n" +
		`<script>__sveltekit_test.resolve(1, () => ["first in, last out"])</script>` + "\n"
	if body != want {
		t.Errorf("chunks\n got %q\nwant %q", body, want)
	}
}

// The engine renders against a promise it cannot wait for, so the value never
// crosses into it — only kit's own placeholder does, wherever the load left the
// promise. `{#await}` renders its pending branch either way
// (svelte/src/internal/server, `await_block`), which is why kit hands its own
// renderer the promise and not the value.
func TestAPromisedValueCrossesIntoTheEngineAsAPromise(t *testing.T) {
	promises := &promiseTable{ids: map[*deferred]int{}}
	node := map[string]any{
		"orderTotal": float64(2),
		// Below the top level, which is where kit lets a promise sit and where
		// skgo used to refuse one.
		"account": map[string]any{"orders": pending()},
	}
	got, err := devalue.StringifyWith(node, []devalue.Reducer{promiseReducer(promises)})
	if err != nil {
		t.Fatalf("stringify: %v", err)
	}
	// kit's own flat form for a reduced value: ["Promise", <slot holding the
	// id>]. The client's reviver reads the id back and makes a promise of it.
	want := `[{"account":1,"orderTotal":4},{"orders":2},["Promise",3],1,2]`
	if got != want {
		t.Errorf("node data\n got %s\nwant %s", got, want)
	}
	if len(promises.order) != 1 {
		t.Errorf("the nested promise was not found: %d in the table, want 1", len(promises.order))
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
