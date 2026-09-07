package skgo

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// silentHandle builds a live server over hook.Intercept(cfg, next) whose
// panics are neither printed by net/http nor by skgo, so a deliberate panic
// in a passing test does not look like a failure. It returns the ids OnPanic
// was told about.
//
// These tests run against a real listener rather than an httptest.Recorder,
// because "the connection was reset" is only observable over a socket — see
// load_panic_test.go, which tests the same property for a server load.
func silentHandle(t *testing.T, hook Handle, next http.Handler) (*httptest.Server, *[]string) {
	t.Helper()
	var reported []string
	cfg := HandleConfig{OnPanic: func(id string, value any, stack []byte) {
		reported = append(reported, id)
	}}
	srv := httptest.NewUnstartedServer(hook.Intercept(cfg, next))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, &reported
}

func notDataHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not data", http.StatusTeapot)
	})
}

// The `handle` hook is application code on the same request, and it runs
// before any load does — a panic there is the whole response.
func TestAPanickingHandleHookAnswersInsteadOfResettingTheConnection(t *testing.T) {
	page := NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{Greeting: "unreachable"}, nil
	})
	ls := mustLoads(t, nil, page)
	hook := Handle(func(ctx context.Context) error { panic("nil locals") })
	srv, reported := silentHandle(t, hook, ls.Intercept(notDataHandler()))

	resp, err := http.Get(srv.URL + "/a/__data.json?x-sveltekit-invalidated=111")
	if err != nil {
		t.Fatalf("the panicking hook killed the connection instead of answering: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// A hook that refuses answers in the shape the refused request expects;
	// a panicking one must answer in exactly that same shape.
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500: %s", resp.StatusCode, body)
	}
	if got := strings.TrimSpace(string(body)); got != `{"status":500,"message":"Internal Error"}` {
		t.Errorf("body = %s, want the opaque 500", got)
	}
	if strings.Contains(string(body), "unreachable") {
		t.Errorf("the load ran even though the hook panicked: %s", body)
	}
	if len(*reported) != 1 || (*reported)[0] != "handle" {
		t.Errorf("OnPanic saw %v, want the hook once", *reported)
	}
}

// A page request is not a data request, but it goes through the same hook.
func TestAPanickingHandleHookDoesNotResetAPageRequest(t *testing.T) {
	ls := mustLoads(t, nil)
	hook := Handle(func(ctx context.Context) error { panic("nil locals") })
	srv, _ := silentHandle(t, hook, ls.Intercept(notDataHandler()))

	resp, err := http.Get(srv.URL + "/a")
	if err != nil {
		t.Fatalf("the panicking hook killed the connection instead of answering: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTeapot {
		t.Fatalf("the request reached the page handler even though the hook panicked")
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500: %s", resp.StatusCode, body)
	}
}
