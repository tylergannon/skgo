// Package account serves the /account section: everything below it is behind
// one rule, and that rule is written here once.
//
// A route directory holds at most one `+layout.server.ts` and one
// `+page.server.ts`, and Go compiles both of their Go files as one package, so
// the two loads cannot both be called `load`. The file a marker sits in is what
// decides which of the two it generates.
package account

import (
	"context"
	"sync/atomic"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// LayoutData is what every page under /account gets, whether it asks or not.
type LayoutData struct {
	// AccountUser is who the visitor is signed in as.
	AccountUser string `json:"accountUser"`
	// AccountSerial counts the times this load has run. The page shows it, so
	// a visitor can see for themselves whether moving between two pages under
	// this layout ran it again.
	AccountSerial int `json:"accountSerial"`
}

var serial atomic.Int64

// layoutLoad is the section's guard and its shared data.
//
// The session comes from the request's locals, which the app's one `handle`
// hook filled in; nothing here reads a cookie or knows how a session is
// spelled. Turning a signed-out visitor away here turns them away from every
// page below, so adding another one needs no new guard.
func layoutLoad(ctx context.Context) (LayoutData, error) {
	session, _ := skgo.LocalOf[businesslogic.Session](ctx)
	if session.User == "" {
		return LayoutData{}, &skgo.Redirect{Status: 307, Location: "/"}
	}

	// The visitor can ask for this section's data again without navigating;
	// `invalidate('app:account')` in the browser is what makes kit's client ask
	// for this node and no other.
	skgo.EventFrom(ctx).Depends("app:account")

	return LayoutData{
		AccountUser:   session.User,
		AccountSerial: int(serial.Add(1)),
	}, nil
}

var _ = skgo.Load(layoutLoad)
