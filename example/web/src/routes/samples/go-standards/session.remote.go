// Package gostandards holds the samples on the Go and web standards page.
package gostandards

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
)

// sessionCookie is the cookie the session id travels in.
const sessionCookie = "skgo_session"

// signedIn reports whether the caller has a live session. It takes only a
// context: the request event travels in it, the way Kit reaches it with
// `getRequestEvent()`, so nothing between the handler and here passes it down.
func signedIn(ctx context.Context) bool {
	id, _ := skgo.EventFrom(ctx).Cookie(sessionCookie)
	return businesslogic.Default.Session(id).User != ""
}

// countTodos counts the todos this visitor may see.
func countTodos(ctx context.Context) (int, error) {
	return len(visitor.Store(ctx).Todos(signedIn(ctx))), nil
}

var _ = skgo.Query(countTodos)
