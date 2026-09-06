package skgo

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A remote function is ordinary Go, so one of them will panic. Kit answers an
// unexpected throw with the same opaque 500 envelope it gives any unexpected
// error, and the visitor gets the app's own error page. Without recovery the
// panic escapes the handler, net/http drops the connection, and the browser
// reports a network failure — one bad handler reads as a server outage.
//
// These tests run against a real listener rather than an httptest.Recorder,
// because "the connection was reset" is only observable over a socket.

// silentServer builds a live server over rs whose panics are not printed, so a
// deliberate panic in a passing test does not look like a failure.
func silentServer(t *testing.T, rs *Remotes) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(rs)
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func TestAPanickingQueryAnswersInsteadOfResettingTheConnection(t *testing.T) {
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "boom",
		func(ctx context.Context, _ None) (todo, error) {
			panic("the database handle was nil")
		}))
	srv := silentServer(t, rs)

	resp, err := http.Get(srv.URL + rs.Prefix() + kithashID(rs, "boom"))
	if err != nil {
		t.Fatalf("the panicking query killed the connection instead of answering: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	kind, _, httpErr := envelope(t, body)
	if kind != "error" {
		t.Fatalf("a panicking query answered %s: %s", kind, body)
	}
	if got := httpErr["status"]; got != float64(500) {
		t.Errorf("status = %v, want 500: %s", got, body)
	}
	// Kit's own answer to an unexpected throw is opaque. The panic's text is
	// the server's business and must not reach the browser.
	if got, _ := httpErr["message"].(string); got != "Internal Error" {
		t.Errorf("message = %q, want %q", got, "Internal Error")
	}
	if strings.Contains(string(body), "database handle") {
		t.Errorf("the panic value reached the client: %s", body)
	}
}

func TestAPanickingCommandAnswersInsteadOfResettingTheConnection(t *testing.T) {
	rs := testRemotes(t, RemoteConfig{}, NewCommand(testModule, "boom",
		func(ctx context.Context, _ None) (todo, error) {
			panic("nil map write")
		}))
	srv := silentServer(t, rs)

	resp, err := http.Post(srv.URL+rs.Prefix()+kithashID(rs, "boom"), "application/json",
		strings.NewReader(`{"payload":"","refreshes":[]}`))
	if err != nil {
		t.Fatalf("the panicking command killed the connection instead of answering: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	kind, _, httpErr := envelope(t, body)
	if kind != "error" || httpErr["status"] != float64(500) {
		t.Fatalf("a panicking command answered %s: %s", kind, body)
	}
}

// A panic in a query a command refreshes must not take the command's own
// answer down with it: the command already ran and may have written a cookie.
func TestAPanickingRefreshDoesNotDestroyTheCommandsAnswer(t *testing.T) {
	command := NewCommand(testModule, "act", func(ctx context.Context, _ None) (string, error) {
		return "done", nil
	})
	query := NewQuery(testModule, "boom", func(ctx context.Context, _ None) (string, error) {
		panic("the refresh exploded")
	})
	rs := testRemotes(t, RemoteConfig{}, command, query)
	srv := silentServer(t, rs)

	body, _ := json.Marshal(map[string]any{
		"payload":   "",
		"refreshes": []string{query.ID() + "/"},
	})
	resp, err := http.Post(srv.URL+rs.Prefix()+command.ID(), "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("a panicking refresh killed the connection: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	kind, data, _ := envelope(t, raw)
	if kind != "result" {
		t.Fatalf("the command's own answer was lost to the refresh: %s", raw)
	}
	if got, _ := object(t, data).Get("_"); got != "done" {
		t.Errorf("the command returned %v, want \"done\"", got)
	}
}

// The producer of a live query runs on its own goroutine, where an unrecovered
// panic does not merely reset one connection — it takes the whole process
// down, every other visitor with it.
func TestAPanickingLiveQueryDoesNotTakeTheProcessDown(t *testing.T) {
	rs := testRemotes(t, RemoteConfig{}, NewLiveQuery(testModule, "boom",
		func(ctx context.Context, _ None, yield func(int) error) error {
			if err := yield(1); err != nil {
				return err
			}
			panic("the subscription exploded")
		}))
	srv := silentServer(t, rs)

	resp, err := http.Get(srv.URL + rs.Prefix() + kithashID(rs, "boom"))
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}
	defer resp.Body.Close()

	frames, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the stream: %v", err)
	}
	if !strings.Contains(string(frames), `"type":"result"`) {
		t.Errorf("the value yielded before the panic never arrived: %s", frames)
	}
	if !strings.Contains(string(frames), `"type":"error"`) {
		t.Errorf("the panicking stream ended without telling the client: %s", frames)
	}
	if strings.Contains(string(frames), "subscription exploded") {
		t.Errorf("the panic value reached the client: %s", frames)
	}
}

// Whatever the client is told, the server operator has to be able to find the
// panic: an opaque 500 with nothing in the log is a bug that cannot be fixed.
func TestAPanicIsReportedToTheServerWithItsStack(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	cfg := RemoteConfig{OnPanic: func(id string, value any, stack []byte) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, id+"|"+string(stack))
		if s, ok := value.(string); ok {
			seen = append(seen, s)
		}
	}}
	rs := testRemotes(t, cfg, NewQuery(testModule, "boom",
		func(ctx context.Context, _ None) (todo, error) {
			panic("the database handle was nil")
		}))
	srv := silentServer(t, rs)

	resp, err := http.Get(srv.URL + rs.Prefix() + kithashID(rs, "boom"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(seen, "\n")
	if !strings.Contains(joined, "/boom") {
		t.Errorf("the report does not name the function: %q", joined)
	}
	if !strings.Contains(joined, "the database handle was nil") {
		t.Errorf("the report does not carry the panic value: %q", joined)
	}
	if !strings.Contains(joined, "skgo.") {
		t.Errorf("the report carries no stack: %q", joined)
	}
}

// kithashID finds a registered function's full id by its export name.
func kithashID(rs *Remotes, name string) string {
	for id := range rs.fns {
		if strings.HasSuffix(id, "/"+name) {
			return id
		}
	}
	return ""
}
