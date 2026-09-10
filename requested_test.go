package skgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tylergannon/polytype/devalue"
)

// The one the issue exists for. The client's list is not a work order: a
// command that says nothing about a query does not run it, however many
// instances of it the request names.
func TestAQueryTheHandlerNeverNamedIsNotRun(t *testing.T) {
	runs := 0
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		runs++
		return todo{ID: id}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		return "renamed", nil
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodoT1, keyGetTodoT2}).Body.Bytes())

	if runs != 0 {
		t.Errorf("the query ran %d times for a handler that never named it, want 0", runs)
	}
	if got := refreshedKeys(t, data); len(got) != 0 {
		t.Errorf("q holds %v, want nothing", got)
	}
	if has(t, data, "r") {
		t.Error("r is set, so the client would believe single-flight updates were performed")
	}
	// The command's own answer is untouched by any of this.
	if got := field(t, data, "_"); got != "renamed" {
		t.Errorf("`_` = %#v, want the command's own result", got)
	}
}

// The cap is the second half of the gate: naming a query does not sign the
// handler up for as many instances of it as the client cares to ask for.
func TestInstancesBeyondTheLimitAreRefusedRatherThanRun(t *testing.T) {
	var ran []string
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		ran = append(ran, id)
		return todo{ID: id, Text: "todo " + id}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		return "renamed", RefreshRequested(ctx, getTodo, 1)
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodoT1, keyGetTodoT2}).Body.Bytes())

	// The first the client named is the one that runs — kit slices the list.
	if !slices.Equal(ran, []string{"t1"}) {
		t.Errorf("the query ran for %v, want only t1", ran)
	}

	// And the one past the limit is failed onto its own key rather than
	// dropped, so the page shows a broken panel instead of a stale one.
	if got := field(t, node(t, data, keyGetTodoT1), "v"); field(t, got, "text") != "todo t1" {
		t.Errorf("t1 = %#v, want the refreshed value", got)
	}
	refused := field(t, node(t, data, keyGetTodoT2), "e")
	if got := field(t, refused, "status"); got != float64(400) {
		t.Errorf("t2 error status = %#v, want 400", got)
	}
	if msg, _ := field(t, refused, "message").(string); !strings.Contains(msg, "at most 1") {
		t.Errorf("t2 error message = %q, want it to say what the limit was", msg)
	}
}

// A limit is per query function, so one busy query cannot spend another's.
func TestTheLimitIsCountedPerQuery(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id}, nil
	}
	getTodos := func(ctx context.Context) ([]string, error) { return []string{"a"}, nil }
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		if err := RefreshRequested(ctx, getTodo, 2); err != nil {
			return "", err
		}
		return "renamed", RefreshRequestedNoArg(ctx, getTodos)
	})
	rs := testRemotes(t, RemoteConfig{},
		NewQuery(testModule, "getTodo", getTodo),
		NewQueryNoArg(testModule, "getTodos", getTodos),
		command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodoT1, keyGetTodos, keyGetTodoT2}).Body.Bytes())

	got := refreshedKeys(t, data)
	slices.Sort(got)
	want := []string{keyGetTodoT1, keyGetTodoT2, keyGetTodos}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("refreshed keys = %v, want %v", got, want)
	}
	for _, key := range want {
		if _, failed := object(t, node(t, data, key)).Get("e"); failed {
			t.Errorf("%s was refused, but it is within its own query's limit", key)
		}
	}
}

// Requested hands the handler the client's arguments so it can decide, and
// refreshes nothing on its own.
func TestRequestedRefreshesNothingUntilTheHandlerSaysSo(t *testing.T) {
	runs := 0
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		runs++
		return todo{ID: id}, nil
	}
	var seen []string
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		requests, err := Requested(ctx, getTodo, 4)
		if err != nil {
			return "", err
		}
		for _, request := range requests {
			seen = append(seen, request.Arg)
			if request.Arg == "t2" {
				request.Refresh()
			}
		}
		return "renamed", nil
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodoT1, keyGetTodoT2}).Body.Bytes())

	// The arguments arrived decoded into the query's own parameter type.
	if !slices.Equal(seen, []string{"t1", "t2"}) {
		t.Errorf("the handler saw %v, want [t1 t2]", seen)
	}
	if runs != 1 {
		t.Errorf("the query ran %d times, want only the one instance the handler took", runs)
	}
	if got := refreshedKeys(t, data); !slices.Equal(got, []string{keyGetTodoT2}) {
		t.Errorf("refreshed keys = %v, want exactly [%s]", got, keyGetTodoT2)
	}
}

// Kit 3.0.0-next.27 distinguishes a deliberate stale value from a forgotten
// requested update. Ignore returns the exact remote key in `i`, without
// running the query or claiming a refresh was performed.
func TestRequestedQueryCanBeExplicitlyIgnored(t *testing.T) {
	runs := 0
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		runs++
		return todo{ID: id}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		requests, err := Requested(ctx, getTodo, 1)
		if err != nil {
			return "", err
		}
		for _, request := range requests {
			request.Ignore()
			request.Ignore() // `i` is a set in Kit, so duplicates collapse.
		}
		return "renamed", nil
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodoT1}).Body.Bytes())

	if runs != 0 {
		t.Errorf("the explicitly ignored query ran %d times, want 0", runs)
	}
	ignored, ok := field(t, data, "i").([]any)
	if !ok || len(ignored) != 1 || ignored[0] != keyGetTodoT1 {
		t.Errorf("i = %#v, want exactly [%s]", ignored, keyGetTodoT1)
	}
	if got := refreshedKeys(t, data); len(got) != 0 {
		t.Errorf("q holds %v, want nothing", got)
	}
	if has(t, data, "r") {
		t.Error("r is set, but ignoring a request performs no refresh")
	}
	if got := field(t, data, "_"); got != "renamed" {
		t.Errorf("`_` = %#v, want the command's own result", got)
	}
}

// An instance whose payload will not decode into the query's parameter type is
// refused on its own key, and the instances beside it are unaffected.
func TestAnUndecodableInstanceFailsAloneAndTheRestRun(t *testing.T) {
	var ran []string
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		ran = append(ran, id)
		return todo{ID: id}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		return "renamed", RefreshRequested(ctx, getTodo, 4)
	})
	rs := testRemotes(t, RemoteConfig{}, NewQuery(testModule, "getTodo", getTodo), command)

	// `WzNd` is kit's payload for the number 3, and getTodo takes a string.
	bad := "worolc/getTodo/WzNd"
	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{bad, keyGetTodoT1}).Body.Bytes())

	if !slices.Equal(ran, []string{"t1"}) {
		t.Errorf("the query ran for %v, want only t1", ran)
	}
	if got := field(t, field(t, node(t, data, bad), "e"), "status"); got != float64(400) {
		t.Errorf("error status = %#v, want 400", got)
	}
	if _, ok := object(t, node(t, data, keyGetTodoT1)).Get("v"); !ok {
		t.Error("the instance beside the bad one was not refreshed")
	}
}

// The mistakes that are about the call rather than about a query are reported
// to the developer instead of quietly accepting or refusing nothing.
func TestRequestedRejectsWhatCannotBeAccepted(t *testing.T) {
	var getTodo queryFn = func(ctx context.Context, id string) (todo, error) {
		return todo{ID: id}, nil
	}
	unregistered := func(ctx context.Context, id string) (todo, error) { return todo{}, nil }
	// A command has a query's shape, so this one is the mistake the compiler
	// cannot catch. (A live query's shape is different, so RefreshRequested on
	// one does not compile in the first place, and neither does
	// ReconnectRequested on a plain query.)
	aCommand := func(ctx context.Context, text string) (todo, error) { return todo{}, nil }

	negative := NewCommandNoArg(testModule, "negative", func(ctx context.Context) (string, error) {
		return "", RefreshRequested(ctx, getTodo, -1)
	})
	notAQuery := NewCommandNoArg(testModule, "notAQuery", func(ctx context.Context) (string, error) {
		return "", RefreshRequested(ctx, aCommand, 1)
	})
	notRegistered := NewCommandNoArg(testModule, "notRegistered", func(ctx context.Context) (string, error) {
		return "", RefreshRequested(ctx, unregistered, 1)
	})
	// A query has nowhere to put a refreshed value, so accepting one from a
	// query is a mistake rather than a no-op.
	fromAQuery := NewQueryNoArg(testModule, "fromAQuery", func(ctx context.Context) (todo, error) {
		return todo{}, RefreshRequested(ctx, getTodo, 1)
	})
	rs := testRemotes(t, RemoteConfig{},
		NewQuery(testModule, "getTodo", getTodo),
		NewCommand(testModule, "aCommand", aCommand),
		negative, notAQuery, notRegistered, fromAQuery)

	for _, tc := range []struct {
		name string
		fn   *Remote
	}{
		{"a negative limit", negative},
		{"a command is not a query", notAQuery},
		{"an unregistered function", notRegistered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, _, httpErr := envelope(t, postCommand(t, rs, tc.fn, devalue.Undefined, []string{keyGetTodoT1}).Body.Bytes())
			if kind != "error" {
				t.Fatalf("type = %q, want error", kind)
			}
			if httpErr["status"] != float64(500) {
				t.Errorf("status = %v, want 500", httpErr["status"])
			}
		})
	}

	t.Run("outside a command or form", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, rs.Prefix()+fromAQuery.ID(), nil)
		rec := httptest.NewRecorder()
		rs.ServeHTTP(rec, req)
		if kind, _, _ := envelope(t, rec.Body.Bytes()); kind != "error" {
			t.Fatalf("type = %q, want error", kind)
		}
	})
}

// The gate holds for a query that takes no argument, which is most of them.
func TestANoArgQueryTheHandlerNeverNamedIsNotRun(t *testing.T) {
	runs := 0
	getTodos := func(ctx context.Context) ([]string, error) {
		runs++
		return []string{"a"}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		return "renamed", nil
	})
	rs := testRemotes(t, RemoteConfig{}, NewQueryNoArg(testModule, "getTodos", getTodos), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodos}).Body.Bytes())

	if runs != 0 {
		t.Errorf("the query ran %d times for a handler that never named it, want 0", runs)
	}
	if got := refreshedKeys(t, data); len(got) != 0 {
		t.Errorf("q holds %v, want nothing", got)
	}
}

// There is one instance of a no-argument query, because kit's client keys it
// under the empty payload. A list that names it twice is still one run.
func TestANoArgQueryIsOneInstanceHoweverOftenItIsNamed(t *testing.T) {
	runs := 0
	getTodos := func(ctx context.Context) ([]string, error) {
		runs++
		return []string{"a"}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		return "renamed", RefreshRequestedNoArg(ctx, getTodos)
	})
	rs := testRemotes(t, RemoteConfig{}, NewQueryNoArg(testModule, "getTodos", getTodos), command)

	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{keyGetTodos, keyGetTodos, keyGetTodos}).Body.Bytes())

	if runs != 1 {
		t.Errorf("the query ran %d times for a list naming it three times, want 1", runs)
	}
	// And the answer on that key is the value, not the refusal a duplicate
	// earned: the instance the page is showing was accepted.
	got, ok := field(t, node(t, data, keyGetTodos), "v").([]any)
	if !ok || len(got) != 1 || got[0] != "a" {
		t.Errorf("%s = %#v, want the refreshed list", keyGetTodos, got)
	}
}

// An argument sent under a no-argument query's id is not an instance of it.
// Kit's validator for a remote function declared without a schema refuses any
// argument at all, and so does this.
func TestAnArgumentSentToANoArgQueryIsRefusedRatherThanRun(t *testing.T) {
	runs := 0
	getTodos := func(ctx context.Context) ([]string, error) {
		runs++
		return []string{"a"}, nil
	}
	command := NewCommandNoArg(testModule, "renameTodo", func(ctx context.Context) (string, error) {
		return "renamed", RefreshRequestedNoArg(ctx, getTodos)
	})
	rs := testRemotes(t, RemoteConfig{}, NewQueryNoArg(testModule, "getTodos", getTodos), command)

	// `WyJ0MSJd` is kit's payload for the string "t1".
	withArg := "worolc/getTodos/WyJ0MSJd"
	_, data, _ := envelope(t, postCommand(t, rs, command, devalue.Undefined,
		[]string{withArg}).Body.Bytes())

	if runs != 0 {
		t.Errorf("the query ran %d times for a key it could not have produced, want 0", runs)
	}
	if got := field(t, field(t, node(t, data, withArg), "e"), "status"); got != float64(400) {
		t.Errorf("error status = %#v, want 400", got)
	}
}
