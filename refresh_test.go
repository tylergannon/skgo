package skgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/devalue"
)

// The keys below were produced by kit itself, not by this package: kit's
// `stringify_remote_arg` and `create_remote_key`, copied verbatim out of
// ephemeral/inspiration/reference/kit/packages/kit/src/runtime/shared.js with
// the transport `encoders` map empty, run against the pinned devalue 5.9.2 in
// ephemeral/inspiration/reference/devalue. The recipe is in the worklog. They
// are the keys the browser holds its query cache under, so they are the keys a
// refresh built in Go has to hit; computing them here with remotearg would let
// every wrong answer agree with itself.
//
// `worolc` is the kit id hash of testModule, "src/lib/todos.remote.ts".
const (
	keyGetTodoT1 = "worolc/getTodo/WyJ0MSJd"
	keyGetTodoT2 = "worolc/getTodo/WyJ0MiJd"
	keyGetTodos  = "worolc/getTodos/"
)

type queryFn = func(context.Context, string) (todo, error)

// refreshedKeys is every key the response's `q` map carries.
func refreshedKeys(t *testing.T, data any) []string {
	t.Helper()
	if !has(t, data, "q") {
		return nil
	}
	return object(t, field(t, data, "q")).Keys()
}

// A refresh names the Go function and its argument, and the key that reaches
// the client is the one kit's own client computed for that argument.
func TestRefreshKeyMatchesKitsForTheArgumentGiven(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id, Text: "text for " + id}, nil
	}
	query := NewQuery(testModule, "getTodo", getTodo)
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		return "renamed", Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{}, query, command)

	kind, data, httpErr := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q (%v), want result", kind, httpErr)
	}

	if got := refreshedKeys(t, data); !slices.Equal(got, []string{keyGetTodoT1}) {
		t.Fatalf("refreshed keys = %v, want exactly [%s]", got, keyGetTodoT1)
	}
	if got := field(t, field(t, node(t, data, keyGetTodoT1), "v"), "text"); got != "text for t1" {
		t.Errorf("refreshed value text = %#v", got)
	}
	// Kit's client reads `r` only in `form.svelte.js`, but its server sets it
	// for any explicit refresh.
	if !has(t, data, "r") {
		t.Error("a response carrying a refresh must set r")
	}
}

// The argument is the key, so refreshing one argument leaves another alone.
// This is the whole reason the argument is typed rather than implied.
func TestRefreshOfOneArgumentDoesNotTouchAnother(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id}, nil
	}
	query := NewQuery(testModule, "getTodo", getTodo)
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		return "renamed", Refresh(ctx, getTodo, "t2")
	})
	rs := testRemotes(t, RemoteConfig{}, query, command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if got := refreshedKeys(t, data); !slices.Equal(got, []string{keyGetTodoT2}) {
		t.Fatalf("refreshed keys = %v, want exactly [%s]", got, keyGetTodoT2)
	}
}

// A query declared with skgo.None takes no argument, and kit's client calls it
// with `undefined`, whose payload is empty. Encoding None as the empty object
// it resembles in Go would key the refresh to something no page holds.
func TestRefreshOfANoArgumentQueryUsesTheEmptyPayload(t *testing.T) {
	getTodos := func(ctx context.Context, _ None) ([]todo, error) {
		return []todo{{ID: "t1"}}, nil
	}
	query := NewQuery(testModule, "getTodos", getTodos)
	command := NewCommand(testModule, "addTodo", func(ctx context.Context, _ None) (string, error) {
		return "added", Refresh(ctx, getTodos, None{})
	})
	rs := testRemotes(t, RemoteConfig{}, query, command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if got := refreshedKeys(t, data); !slices.Equal(got, []string{keyGetTodos}) {
		t.Fatalf("refreshed keys = %v, want exactly [%s]", got, keyGetTodos)
	}
}

// Kit defers the refresh until the handler body has finished, so the value the
// client is sent is the one the command left behind and not one read halfway
// through it.
func TestRefreshRunsAfterTheHandlerBody(t *testing.T) {
	stored := "before the command"
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id, Text: stored}, nil
	}
	query := NewQuery(testModule, "getTodo", getTodo)
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		if err := Refresh(ctx, getTodo, "t1"); err != nil {
			return "", err
		}
		// Everything the command does after asking still has to reach the
		// client.
		stored = "after the command"
		return "renamed", nil
	})
	rs := testRemotes(t, RemoteConfig{}, query, command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if got := field(t, field(t, node(t, data, keyGetTodoT1), "v"), "text"); got != "after the command" {
		t.Fatalf("refreshed text = %#v, want the value the command left behind", got)
	}
}

// A query refreshed by a command runs on an event derived from the command's,
// sharing its cookie jar, so it sees the cookie the command just wrote.
func TestRefreshSeesACookieTheCommandJustWrote(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		user, _ := EventFrom(ctx).Cookie("who")
		return todo{ID: id, Text: user}, nil
	}
	query := NewQuery(testModule, "getTodo", getTodo)
	command := NewCommand(testModule, "signIn", func(ctx context.Context, _ None) (string, error) {
		if err := EventFrom(ctx).SetCookie("who", "ada", CookieOptions{}); err != nil {
			return "", err
		}
		return "signed in", Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{}, query, command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if got := field(t, field(t, node(t, data, keyGetTodoT1), "v"), "text"); got != "ada" {
		t.Fatalf("refreshed text = %#v, want the cookie the command wrote", got)
	}
}

// A refresh that fails is reported on that query's key. The command keeps its
// own answer, exactly as kit's does.
func TestRefreshFailureDoesNotFailTheCommand(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{}, Errorf(404, "No todo with id %q", id)
	}
	query := NewQuery(testModule, "getTodo", getTodo)
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		return "the command's own answer", Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{}, query, command)

	kind, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if kind != "result" {
		t.Fatalf("type = %q, want result: a failed refresh must not fail the command", kind)
	}
	if got := field(t, data, "_"); got != "the command's own answer" {
		t.Errorf("command result = %#v", got)
	}
	n := node(t, data, keyGetTodoT1)
	if has(t, n, "v") {
		t.Error("a failed refresh must not carry a value")
	}
	if got := field(t, field(t, n, "e"), "status"); got != float64(404) {
		t.Errorf("refresh error status = %#v, want 404", got)
	}
}

// A refreshed query may itself ask for a refresh, so the drain has to keep
// going until nothing new is registered — kit's re-entrant `drain()`.
func TestARefreshedQueryCanRegisterAnother(t *testing.T) {
	getTodos := func(ctx context.Context, _ None) ([]todo, error) {
		return []todo{{ID: "t1"}}, nil
	}
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id}, Refresh(ctx, getTodos, None{})
	}
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		return "renamed", Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{},
		NewQuery(testModule, "getTodo", getTodo),
		NewQuery(testModule, "getTodos", getTodos),
		command,
	)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, nil).Body.Bytes())
	if got := refreshedKeys(t, data); !slices.Equal(got, []string{keyGetTodoT1, keyGetTodos}) {
		t.Fatalf("refreshed keys = %v, want [%s %s]", got, keyGetTodoT1, keyGetTodos)
	}
}

// The Go handler's own refresh is the server's instruction; a client that
// asked for the same key does not get to duplicate it or re-run the query.
func TestOneKeyNamedByBothChannelsIsAnsweredOnce(t *testing.T) {
	runs := 0
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		runs++
		return todo{ID: id}, nil
	}
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		return "renamed", Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, []string{keyGetTodoT1}).Body.Bytes())
	if got := refreshedKeys(t, data); !slices.Equal(got, []string{keyGetTodoT1}) {
		t.Fatalf("refreshed keys = %v, want exactly [%s]", got, keyGetTodoT1)
	}
	if runs != 1 {
		t.Errorf("the query ran %d times, want 1", runs)
	}
}

// A command may still refresh a key the client did not name, and a client key
// the handler said nothing about is still answered.
func TestBothChannelsAreAnsweredTogether(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id}, nil
	}
	command := NewCommand(testModule, "renameTodo", func(ctx context.Context, _ None) (string, error) {
		return "renamed", Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined, []string{keyGetTodoT2}).Body.Bytes())
	got := refreshedKeys(t, data)
	slices.Sort(got)
	if want := []string{keyGetTodoT1, keyGetTodoT2}; !slices.Equal(got, want) {
		t.Fatalf("refreshed keys = %v, want %v", got, want)
	}
}

// Refresh reports the mistakes that are about the call rather than about the
// query, instead of silently updating nothing.
func TestRefreshRejectsWhatCannotBeRefreshed(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id}, nil
	}
	unregistered := func(ctx context.Context, id string) (todo, error) { return todo{}, nil }

	var self func(context.Context, None) (string, error)
	self = func(ctx context.Context, _ None) (string, error) {
		return "", Refresh(ctx, self, None{})
	}
	notAQuery := NewCommand(testModule, "notAQuery", self)

	notRegistered := NewCommand(testModule, "notRegistered", func(ctx context.Context, _ None) (string, error) {
		return "", Refresh(ctx, unregistered, "t1")
	})
	// A query has nowhere to put a refreshed value, so asking from one is a
	// mistake rather than a no-op.
	fromAQuery := NewQuery(testModule, "fromAQuery", func(ctx context.Context, _ None) (todo, error) {
		return todo{}, Refresh(ctx, getTodo, "t1")
	})
	rs := testRemotes(t, RemoteConfig{},
		NewQuery(testModule, "getTodo", getTodo), notAQuery, notRegistered, fromAQuery)

	for _, tc := range []struct {
		name string
		fn   *Remote
	}{
		{"a command is not a query", notAQuery},
		{"an unregistered function", notRegistered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, _, httpErr := envelope(t, postCommand(t, rs, tc.fn, devalue.Undefined, nil).Body.Bytes())
			if kind != "error" {
				t.Fatalf("type = %q, want error", kind)
			}
			// What the client is told is opaque; the developer's copy is the
			// error the handler returned, which becomes an ordinary 500.
			if httpErr["status"] != float64(500) {
				t.Errorf("status = %v, want 500", httpErr["status"])
			}
		})
	}

	t.Run("outside a command", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, rs.Prefix()+fromAQuery.ID(), nil)
		rec := httptest.NewRecorder()
		rs.ServeHTTP(rec, req)
		kind, _, _ := envelope(t, rec.Body.Bytes())
		if kind != "error" {
			t.Fatalf("type = %q, want error", kind)
		}
	})
}

// Two registrations of one Go function cannot be told apart by a refresh, so
// the registry refuses to start rather than refreshing whichever one won.
func TestNewRemotesRefusesTwoRegistrationsOfOneFunction(t *testing.T) {
	shared := func(ctx context.Context, id string) (todo, error) { return todo{ID: id}, nil }
	_, err := NewRemotes(RemoteConfig{},
		NewQuery(testModule, "getTodo", shared),
		NewQuery(testModule, "getOther", shared),
	)
	if err == nil {
		t.Fatal("NewRemotes accepted two registrations of one Go function")
	}
	if !strings.Contains(err.Error(), "same Go function") {
		t.Errorf("error = %v", err)
	}
}
