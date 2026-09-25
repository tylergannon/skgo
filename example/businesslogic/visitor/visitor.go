// Package visitor decides which visitor a request belongs to, and therefore
// whose state it reads and writes.
//
// A visitor is a random id in a cookie. The root layout's load hands one out
// the first time a browser asks for a page, because a load is where kit lets a
// page request write a cookie and the root layout is on every page. Every
// other reader — a remote function, a server route, another load — reads the
// id back and never writes it.
//
// Only a browser is handed one. A request that carries no visitor cookie and
// did not come from a browser — `curl`, the generated Go client, a Go test
// driving the handler — belongs to nobody, and nobody's state is
// businesslogic.Default: the one list such callers have always shared, and
// still share, across as many requests as they make.
package visitor

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// Cookie is the name the visitor id travels under.
const Cookie = "skgo_visitor"

// id is the request local Start leaves behind. It exists for the one request
// that has no cookie yet: the root layout's load mints the id and writes it on
// the response, but the queries the same document then renders read the
// request, which was sent before the cookie existed. Kit's `cookies.get`
// answers with a cookie set earlier in the same request; the local is how this
// request's remote functions get the same answer.
type id string

// Start makes sure a browser has a visitor id, writing a new one on the
// response if the request carried none. The root layout's load calls it.
//
// A browser is a request with a Sec-Fetch-Site header, which browsers send on
// every request to a secure origin (localhost counts as one) and which a page's
// script cannot set. Nothing finer is needed: a caller that sends neither that
// header nor the cookie is exactly the caller Default exists for, and handing
// it an id it will never send back would move this one request off the list
// it wrote to a moment ago.
func Start(ctx context.Context) error {
	event := skgo.EventFrom(ctx)
	visitor, ok := event.Cookie(Cookie)
	if !ok || visitor == "" {
		if event.Request().Header.Get("Sec-Fetch-Site") == "" {
			return nil
		}
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		visitor = base64.RawURLEncoding.EncodeToString(raw)
		if err := event.SetCookie(Cookie, visitor, skgo.CookieOptions{Path: "/"}); err != nil {
			return err
		}
	}
	return skgo.SetLocal(ctx, id(visitor))
}

// Of is the visitor this request belongs to, or "" for nobody.
func Of(ctx context.Context) string {
	if visitor, ok := skgo.LocalOf[id](ctx); ok {
		return string(visitor)
	}
	visitor, _ := skgo.EventFrom(ctx).Cookie(Cookie)
	return visitor
}

// Store is the todo store of the visitor this request belongs to.
func Store(ctx context.Context) *businesslogic.Store {
	return businesslogic.For(Of(ctx))
}
