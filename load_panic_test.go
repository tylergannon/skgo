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

// A server load is ordinary Go, so one of them will panic. It matters more
// here than on a remote call: the branch's loads each run on their own
// goroutine, and an unrecovered panic on a goroutine is not one reset
// connection but a dead process.
//
// These tests run against a real listener rather than an httptest.Recorder,
// because "the connection was reset" is only observable over a socket.

// silentLoads builds a live server over ls whose panics are neither printed by
// net/http nor by skgo, so a deliberate panic in a passing test does not look
// like a failure. It returns the ids OnPanic was told about.
func silentLoads(t *testing.T, tweak func(*LoadConfig), loads ...*ServerLoad) (*httptest.Server, *[]string) {
	t.Helper()
	var reported []string
	ls := mustLoads(t, func(cfg *LoadConfig) {
		if tweak != nil {
			tweak(cfg)
		}
		cfg.OnPanic = func(id string, value any, stack []byte) {
			reported = append(reported, id)
		}
	}, loads...)

	srv := httptest.NewUnstartedServer(ls.Intercept(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not data", http.StatusTeapot) })))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, &reported
}

func TestAPanickingLoadAnswersInsteadOfResettingTheConnection(t *testing.T) {
	srv, reported := silentLoads(t, nil,
		NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
			panic("the database handle was nil")
		}))

	resp, err := http.Get(srv.URL + "/a/__data.json?x-sveltekit-invalidated=111")
	if err != nil {
		t.Fatalf("the panicking load killed the connection instead of answering: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Kit answers a load's unexpected throw at HTTP 200 with an error node, so
	// its client renders the app's error page rather than reloading.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	want := `{"type":"data","nodes":[null,null,{"type":"error","error":{"status":500,"message":"Internal Error"}}]}`
	if got := strings.TrimSpace(string(body)); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
	// The panic's text is the server's business; the record it leaves is
	// OnPanic, and that is the only place it may appear.
	if strings.Contains(string(body), "database handle") {
		t.Errorf("the panic value reached the client: %s", body)
	}
	if len(*reported) != 1 || (*reported)[0] != "src/routes/a/+page.server.ts" {
		t.Errorf("OnPanic saw %v, want the panicking load's module once", *reported)
	}
}

// The sibling matters: a panic on one node's goroutine must not take the
// branch's other nodes with it, and the client must still get their data.
func TestAPanickingLoadDoesNotTakeItsSiblingsDown(t *testing.T) {
	srv, _ := silentLoads(t, nil,
		NewLoad("src/routes/a/+layout.server.ts", func(ctx context.Context) (layoutData, error) {
			return layoutData{Who: "ada"}, nil
		}),
		NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
			panic("index out of range")
		}))

	resp, err := http.Get(srv.URL + "/a/__data.json?x-sveltekit-invalidated=111")
	if err != nil {
		t.Fatalf("the panicking load killed the connection instead of answering: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	want := `{"type":"data","nodes":[null,{"type":"data","data":[{"who":1},"ada"],"uses":{}},{"type":"error","error":{"status":500,"message":"Internal Error"}}],"uses":{}}`
	_ = want
	got := strings.TrimSpace(string(body))
	if !strings.Contains(got, `"ada"`) {
		t.Errorf("the layout's data did not survive its sibling's panic: %s", got)
	}
	if !strings.Contains(got, `{"type":"error","error":{"status":500,"message":"Internal Error"}}`) {
		t.Errorf("the panicking page was not answered as an error node: %s", got)
	}
}

// The `handle` hook is application code on the same request, and it runs
// before any load does — a panic there is the whole response.
func TestAPanickingHandleHookAnswersInsteadOfResettingTheConnection(t *testing.T) {
	srv, reported := silentLoads(t, func(cfg *LoadConfig) {
		cfg.Handle = func(ctx context.Context) error { panic("nil locals") }
	}, NewLoad("src/routes/a/+page.server.ts", func(ctx context.Context) (pageData, error) {
		return pageData{Greeting: "unreachable"}, nil
	}))

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
	srv, _ := silentLoads(t, func(cfg *LoadConfig) {
		cfg.Handle = func(ctx context.Context) error { panic("nil locals") }
	})

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
