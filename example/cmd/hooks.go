package main

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// sessionCookie is the cookie the session id travels in. This is the one place
// the app spells it: everything downstream reads the session, not the cookie.
const sessionCookie = "skgo_session"

// handle is the app's `handle` hook — the one place it decides who the caller
// is. It runs once per request, before any load or remote function, and puts
// the answer where all of them can read it with skgo.LocalOf.
//
// A guard is then a load that reads the session; see
// web/src/routes/account/layout.server.go, which turns a signed-out visitor
// away from every page under /account without any of those pages knowing.
func handle(ctx context.Context) error {
	id, _ := skgo.EventFrom(ctx).Cookie(sessionCookie)
	return skgo.SetLocal(ctx, businesslogic.Default.Session(id))
}
