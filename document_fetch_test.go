package skgo

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

// fetchFixture answers GET with a fixed JSON body and echoes a request header
// back, so a test can tell that the request Go actually dispatched carried
// what the render asked for.
func fetchFixture() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Saw-Cookie", r.Header.Get("Cookie"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"hello from Go"}`))
	})
}

// TestFetchDispatchRunsInProcess is the proof that a render-time fetch never
// opens a socket: the same http.Handler a real request would reach answers
// one built from a JSON envelope, with no listener anywhere in the test.
func TestFetchDispatchRunsInProcess(t *testing.T) {
	s := &SSR{fetch: fetchFixture()}

	answer := s.fetchAnswer(context.Background(), ssr.FetchRequest{
		Method:  "GET",
		URL:     "http://example.test/render-fetch/greeting",
		Headers: map[string]string{"Cookie": "skgo_session=abc123"},
	})

	if answer.Error != "" {
		t.Fatalf("fetchAnswer refused: %s", answer.Error)
	}
	if answer.Response == nil {
		t.Fatal("fetchAnswer returned neither a response nor an error")
	}
	if answer.Response.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", answer.Response.Status)
	}
	if !strings.Contains(answer.Response.Body, `"message":"hello from Go"`) {
		t.Errorf("body = %q", answer.Response.Body)
	}
	if got := answer.Response.Headers["X-Saw-Cookie"]; got != "skgo_session=abc123" {
		t.Errorf("the dispatched request did not carry the cookie the render sent; handler saw %q", got)
	}
}

// TestFetchDispatchWithNoRouteRefuses is what an app with no server-route
// registry gets: a clear refusal rather than a nil-pointer panic.
func TestFetchDispatchWithNoRouteRefuses(t *testing.T) {
	s := &SSR{}

	answer := s.fetchAnswer(context.Background(), ssr.FetchRequest{Method: "GET", URL: "http://example.test/nowhere"})
	if answer.Response != nil {
		t.Fatalf("got a response with no Fetch handler configured: %+v", answer.Response)
	}
	if answer.Error == "" {
		t.Fatal("no error message for a render-time fetch with nothing to answer it")
	}
}

// TestFetchDispatchCarriesA404Through is the other half: a same-origin fetch
// to a path with no `+server.ts` is a real 404 response, the same one a
// browser making the same request would see — not a thrown error, because the
// fetch itself succeeded.
func TestFetchDispatchCarriesA404Through(t *testing.T) {
	s := &SSR{fetch: http.NotFoundHandler()}

	answer := s.fetchAnswer(context.Background(), ssr.FetchRequest{Method: "GET", URL: "http://example.test/no-such-route"})
	if answer.Error != "" {
		t.Fatalf("a 404 from the handler was reported as a fetch failure: %s", answer.Error)
	}
	if answer.Response == nil || answer.Response.Status != http.StatusNotFound {
		t.Fatalf("response = %+v, want a 404", answer.Response)
	}
}

// TestMatchDispatchFindsTheSameRouteEndpointsAnswers pins match() to the exact
// route table a `+server.ts` request matches against — Go's own, not a second
// router built for the engine.
func TestMatchDispatchFindsTheSameRouteEndpointsAnswers(t *testing.T) {
	loads, err := NewLoads(LoadConfig{
		Routes: []ManifestRoute{
			{ID: "/api/todos", Pattern: `^\/api\/todos\/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}},
			{ID: "/items/[id]", Pattern: `^\/items\/([^/]+?)\/?$`, Params: []ManifestParam{{Name: "id"}},
				Page: &ManifestPage{Layouts: []int{}, Leaf: 0}},
		},
	})
	if err != nil {
		t.Fatalf("NewLoads: %v", err)
	}
	s := &SSR{loads: loads}

	id, params, ok := s.matchDispatch("/api/todos")
	if !ok || id != "/api/todos" || len(params) != 0 {
		t.Errorf("match(/api/todos) = %q, %v, %v; want /api/todos, {}, true", id, params, ok)
	}

	id, params, ok = s.matchDispatch("/items/93")
	if !ok || id != "/items/[id]" || params["id"] != "93" {
		t.Errorf("match(/items/93) = %q, %v, %v; want /items/[id], {id: 93}, true", id, params, ok)
	}

	if _, _, ok := s.matchDispatch("/no-such-route"); ok {
		t.Error("match(/no-such-route) matched something")
	}
}

func TestRenderFetchCarriesCancellationToTheGoHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	done := make(chan struct{})
	s := &SSR{fetch: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		w.WriteHeader(http.StatusRequestTimeout)
	})}
	go func() { defer close(done); s.fetchAnswer(ctx, ssr.FetchRequest{URL: "http://example.test/wait"}) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("render fetch lost its cancellation context")
	}
}
