package skgo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/remotearg"
)

// payload is one argument as kit's client sends it: devalue's flat form,
// base64url-encoded, which is also the string the cache key is built from.
func payload(t *testing.T, arg any) string {
	t.Helper()
	out, err := remotearg.StringifyQueryArg(arg)
	if err != nil {
		t.Fatalf("StringifyQueryArg(%#v): %v", arg, err)
	}
	return out
}

type tickerQuote struct {
	Symbol    string `json:"symbol"`
	Cents     int    `json:"cents"`
	BatchSize int    `json:"batchSize"`
}

// batchQuotes is the registration every test here uses: it reports the batch it
// was handed, so a caller can tell one call of four from four calls of one.
func batchQuotes(seen *[][]string) *Remote {
	return NewBatchQuery(testModule, "getQuotes", func(ctx context.Context, symbols []string) ([]tickerQuote, error) {
		if seen != nil {
			*seen = append(*seen, append([]string(nil), symbols...))
		}
		out := make([]tickerQuote, len(symbols))
		for i, symbol := range symbols {
			if symbol == "NOPE" {
				return nil, Errorf(404, "No quote for %q", symbol)
			}
			out[i] = tickerQuote{Symbol: symbol, Cents: 100 * (i + 1), BatchSize: len(symbols)}
		}
		return out, nil
	})
}

// post is a batch request the way kit's client sends one:
// `POST /_app/remote/<id>` with `{"payloads": [...]}`.
func post(t *testing.T, rs *Remotes, fn *Remote, payloads ...string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"payloads": payloads})
	if err != nil {
		t.Fatalf("marshalling payloads: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+fn.ID(), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	return rec
}

// nodes returns the `_` array kit's client resolves its batched promises from.
func nodes(t *testing.T, data any) []any {
	t.Helper()
	list, ok := field(t, data, "_").([]any)
	if !ok {
		t.Fatalf("`_` is %#v, want a list", field(t, data, "_"))
	}
	return list
}

func TestBatchQueryAnswersEveryPayloadInOneCall(t *testing.T) {
	var seen [][]string
	getQuotes := batchQuotes(&seen)
	rs := testRemotes(t, RemoteConfig{Version: "v1"}, getQuotes)

	// devalue payloads for "SKGO", "GOJA" and "KITX", base64url of ["<symbol>"].
	rec := post(t, rs, getQuotes, payload(t, "SKGO"), payload(t, "GOJA"), payload(t, "KITX"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("cache-control = %q", got)
	}
	if got := rec.Header().Get("X-Sveltekit-Version"); got != "v1" {
		t.Errorf("x-sveltekit-version = %q, want v1", got)
	}

	// One call, three arguments. That is the whole point of a batch query, and
	// three calls of one would answer identically without it.
	if len(seen) != 1 {
		t.Fatalf("the Go function ran %d times, want 1: %v", len(seen), seen)
	}
	if got := strings.Join(seen[0], ","); got != "SKGO,GOJA,KITX" {
		t.Errorf("the batch was %q, want SKGO,GOJA,KITX", got)
	}

	kind, data, _ := envelope(t, rec.Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result", kind)
	}

	// Positional: kit's client matches result i to payload i.
	list := nodes(t, data)
	if len(list) != 3 {
		t.Fatalf("`_` has %d nodes, want 3", len(list))
	}
	for i, want := range []string{"SKGO", "GOJA", "KITX"} {
		if got := field(t, list[i], "type"); got != "result" {
			t.Errorf("node %d type = %#v, want result", i, got)
		}
		got := field(t, list[i], "data")
		if symbol := field(t, got, "symbol"); symbol != want {
			t.Errorf("node %d symbol = %#v, want %q", i, symbol, want)
		}
		if size := field(t, got, "batchSize"); size != float64(3) {
			t.Errorf("node %d batchSize = %#v, want 3", i, size)
		}
	}

	// A batch query's values reach the client through `_` alone: its client
	// resolves the promises it is holding rather than seeding a cache, so kit
	// puts nothing under `q` here.
	if has(t, data, "q") {
		t.Errorf("a batch response must not carry q; keys are %v", object(t, data).Keys())
	}
}

func TestBatchQueryFailsTheWholeCall(t *testing.T) {
	getQuotes := batchQuotes(nil)
	rs := testRemotes(t, RemoteConfig{}, getQuotes)

	rec := post(t, rs, getQuotes, payload(t, "SKGO"), payload(t, "NOPE"))

	kind, _, failure := envelope(t, rec.Body.Bytes())
	if kind != "error" {
		t.Fatalf("type = %q, want error", kind)
	}
	if got := failure["status"]; got != float64(404) {
		t.Errorf("status = %#v, want 404", got)
	}
	if got := failure["message"]; got != `No quote for "NOPE"` {
		t.Errorf("message = %#v", got)
	}
}

func TestBatchQueryRefusesGet(t *testing.T) {
	getQuotes := batchQuotes(nil)
	rs := testRemotes(t, RemoteConfig{}, getQuotes)

	req := httptest.NewRequest(http.MethodGet, rs.Prefix()+getQuotes.ID(), nil)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	_, _, failure := envelope(t, rec.Body.Bytes())
	if got, _ := failure["message"].(string); !strings.Contains(got, "must be invoked via POST") {
		t.Errorf("message = %q", got)
	}
}

func TestBatchQueryRefusesAnUndecodablePayload(t *testing.T) {
	getQuotes := batchQuotes(nil)
	rs := testRemotes(t, RemoteConfig{}, getQuotes)

	rec := post(t, rs, getQuotes, "not-a-payload")

	_, _, failure := envelope(t, rec.Body.Bytes())
	if got := failure["status"]; got != float64(400) {
		t.Errorf("status = %#v, want 400", got)
	}
}

// A batch whose function answers a different number of results than it was
// asked questions has no honest way to match them up. The client gets kit's
// opaque 500 and the detail stays on the server.
func TestBatchQueryWithMismatchedResultsIsAnInternalError(t *testing.T) {
	short := NewBatchQuery(testModule, "getQuotes", func(ctx context.Context, symbols []string) ([]tickerQuote, error) {
		return []tickerQuote{{Symbol: symbols[0]}}, nil
	})
	rs := testRemotes(t, RemoteConfig{}, short)

	rec := post(t, rs, short, payload(t, "SKGO"), payload(t, "GOJA"))

	_, _, failure := envelope(t, rec.Body.Bytes())
	if got := failure["status"]; got != float64(500) {
		t.Errorf("status = %#v, want 500", got)
	}
	if got := failure["message"]; got != "Internal Error" {
		t.Errorf("message = %#v, want Internal Error", got)
	}
}

// A Refresh names one instance of a query, and a batch query is a query: kit's
// own `refresh()` stores the resource's promise, which went through `enqueue`
// and became a batch with one entry in it.
func TestBatchQueryAnswersASingleRefreshedInstance(t *testing.T) {
	var seen [][]string
	getQuotes := batchQuotes(&seen)
	rs := testRemotes(t, RemoteConfig{}, getQuotes)
	value, err := rs.call(context.Background(), getQuotes, "SKGO", true)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(seen) != 1 || len(seen[0]) != 1 || seen[0][0] != "SKGO" {
		t.Fatalf("the batch was %v, want one call of [SKGO]", seen)
	}
	// call answers with the tree the transport hook produced, which is what
	// goes on the wire and into a document.
	got, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want the encoded tree", value)
	}
	if got["symbol"] != "SKGO" {
		t.Errorf("symbol = %#v, want SKGO", got["symbol"])
	}
	if got["batchSize"] != float64(1) {
		t.Errorf("batchSize = %#v, want 1", got["batchSize"])
	}
}
