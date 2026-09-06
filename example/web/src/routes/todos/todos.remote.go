// Package todos holds the server logic for the /todos routes, written in Go
// and colocated with the pages that use it.
//
// `skgo generate` reads the declarations at the bottom of this file and emits
// todos.remote.ts beside it. Nothing in this directory is hand-written
// TypeScript.
package todos

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// sessionCookie is the name of the cookie the auth functions in src/lib set.
const sessionCookie = "skgo_session"

// signedIn reports whether the caller has a live session. The request is
// reachable from the context, the way kit reaches it with `getRequestEvent()`.
// A query may read cookies; only a command may write them.
func signedIn(ctx context.Context) bool {
	id, _ := skgo.EventFrom(ctx).Cookie(sessionCookie)
	return businesslogic.Default.Session(id).User != ""
}

// getTodos lists the todos this visitor may see.
func getTodos(ctx context.Context, _ skgo.None) ([]businesslogic.Todo, error) {
	return businesslogic.Default.Todos(signedIn(ctx)), nil
}

// getTodo looks one todo up by id.
func getTodo(ctx context.Context, id string) (businesslogic.Todo, error) {
	todo, ok := businesslogic.Default.Todo(id, signedIn(ctx))
	if !ok {
		return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", id)
	}
	return todo, nil
}

// addTodo appends a todo to the list.
func addTodo(ctx context.Context, text string) (businesslogic.Todo, error) {
	if text == "" {
		return businesslogic.Todo{}, skgo.Errorf(400, "A todo needs some text")
	}
	return businesslogic.Default.Add(text), nil
}

// Rename is the argument of the renameTodo command.
type Rename struct {
	// ID names the todo to change.
	ID string `json:"id"`
	// Text is its new text.
	Text string `json:"text"`
}

// renameTodo changes one todo's text.
func renameTodo(ctx context.Context, arg Rename) (businesslogic.Todo, error) {
	todo, ok := businesslogic.Default.Rename(arg.ID, arg.Text)
	if !ok {
		return businesslogic.Todo{}, skgo.Errorf(404, "No todo with id %q", arg.ID)
	}
	return todo, nil
}

// watchCount pushes the number of todos this visitor may see, now and after
// every change, until the client disconnects.
//
// The session is fixed when the stream opens, the way kit's own live queries
// work: signing in or out gives the page a different visitor, and the page
// re-subscribes.
func watchCount(ctx context.Context, _ skgo.None, yield func(int) error) error {
	updates, unsubscribe, count := businesslogic.Default.Watch(signedIn(ctx))
	defer unsubscribe()

	if err := yield(count); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-updates:
			if err := yield(n); err != nil {
				return err
			}
		}
	}
}

var (
	_ = skgo.Query(getTodos)
	_ = skgo.Query(getTodo)
	_ = skgo.Command(addTodo)
	_ = skgo.Command(renameTodo)
	_ = skgo.LiveQuery(watchCount)
)
