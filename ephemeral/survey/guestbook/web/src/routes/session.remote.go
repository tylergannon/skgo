// Package routes holds the guestbook's root-level server logic in Go.
package routes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/survey/store"
)

const sessionCookie = "session"
const visitsCookie = "visits"

// Session is what the layout shows: who the visitor is and how many times
// they have arrived.
type Session struct {
	User   string `json:"user"`
	Visits int    `json:"visits"`
}

// getSession is the read-only half of the junkyard app's +layout.server.ts.
// The other half — incrementing the `visits` cookie — cannot live here: kit
// forbids a query from writing cookies, and skgo enforces the same rule.
func getSession(ctx context.Context, _ skgo.None) (Session, error) {
	e := skgo.EventFrom(ctx)
	user, _ := e.Cookie(sessionCookie)
	raw, _ := e.Cookie(visitsCookie)
	visits, _ := strconv.Atoi(raw)
	return Session{User: user, Visits: visits}, nil
}

// recordVisit is the cookie-writing half, demoted to a command because that is
// the only kind of remote function allowed to set a cookie.
func recordVisit(ctx context.Context, _ skgo.None) (Session, error) {
	e := skgo.EventFrom(ctx)
	user, _ := e.Cookie(sessionCookie)
	raw, _ := e.Cookie(visitsCookie)
	visits, _ := strconv.Atoi(raw)
	visits++
	if err := e.SetCookie(visitsCookie, strconv.Itoa(visits), skgo.CookieOptions{Path: "/"}); err != nil {
		return Session{}, err
	}
	return Session{User: user, Visits: visits}, nil
}

// SignInResult mirrors what the junkyard action returned through `form`.
type SignInResult struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// signIn replaces the `login` form action.
func signIn(ctx context.Context, name string) (SignInResult, error) {
	if name == "" {
		return SignInResult{Error: "name is required"}, nil
	}
	e := skgo.EventFrom(ctx)
	if err := e.SetCookie(sessionCookie, name, skgo.CookieOptions{Path: "/"}); err != nil {
		return SignInResult{}, err
	}
	return SignInResult{Name: name}, nil
}

// signOut replaces the `logout` form action.
func signOut(ctx context.Context, _ skgo.None) (skgo.None, error) {
	e := skgo.EventFrom(ctx)
	return skgo.None{}, e.DeleteCookie(sessionCookie, skgo.CookieOptions{Path: "/"})
}

// Whoami is the payload of the /whoami page.
type Whoami struct {
	URL           string `json:"url"`
	ClientAddress string `json:"clientAddress"`
	RequestID     string `json:"requestId"`
	User          string `json:"user"`
	Host          string `json:"host"`
}

// whoami answers what the junkyard app's whoami load answered: the page's own
// URL, the caller's address, a per-request id set by a hook, and the Host
// header.
func whoami(ctx context.Context, _ skgo.None) (Whoami, error) {
	e := skgo.EventFrom(ctx)
	r := e.Request()
	user, _ := e.Cookie(sessionCookie)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return Whoami{
		URL:           r.URL.String(),
		ClientAddress: r.RemoteAddr,
		RequestID:     hex.EncodeToString(id),
		User:          user,
		Host:          r.Host,
	}, nil
}

// Home is the root page's data.
type Home struct {
	Greeting     string `json:"greeting"`
	MessageCount int    `json:"messageCount"`
}

// getHome replaces the root +page.server.ts load.
func getHome(ctx context.Context, _ skgo.None) (Home, error) {
	return Home{Greeting: "hello from the server", MessageCount: store.Default.Count()}, nil
}

var (
	_ = skgo.Query(getSession)
	_ = skgo.Command(recordVisit)
	_ = skgo.Command(signIn)
	_ = skgo.Command(signOut)
	_ = skgo.Query(whoami)
	_ = skgo.Query(getHome)
)
