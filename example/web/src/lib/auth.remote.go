// Package lib holds server logic shared by more than one route.
package lib

import (
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// sessionCookie is the cookie the session id travels in.
const sessionCookie = "skgo_session"

// whoami reports who the caller is signed in as. A query may read cookies but
// never write them, so this cannot accidentally start a session.
func whoami(e *skgo.Event, _ skgo.None) (businesslogic.Session, error) {
	id, _ := e.Cookie(sessionCookie)
	return businesslogic.Default.Session(id), nil
}

// signIn opens a session and stores its id in an HttpOnly cookie. Kit allows
// cookie writes in commands and nowhere else, which is why this takes a
// *skgo.CommandEvent.
func signIn(e *skgo.CommandEvent, user string) (businesslogic.Session, error) {
	if user == "" {
		return businesslogic.Session{}, skgo.Errorf(400, "Who are you?")
	}
	id := businesslogic.Default.SignIn(user)
	if err := e.SetCookie(sessionCookie, id, skgo.CookieOptions{MaxAge: 60 * 60}); err != nil {
		return businesslogic.Session{}, err
	}
	return businesslogic.Session{User: user}, nil
}

// signOut ends the session and clears the cookie.
func signOut(e *skgo.CommandEvent, _ skgo.None) (businesslogic.Session, error) {
	id, _ := e.Cookie(sessionCookie)
	businesslogic.Default.SignOut(id)
	if err := e.DeleteCookie(sessionCookie, skgo.CookieOptions{}); err != nil {
		return businesslogic.Session{}, err
	}
	return businesslogic.Session{}, nil
}

var (
	_ = skgo.Query(whoami)
	_ = skgo.Command(signIn)
	_ = skgo.Command(signOut)
)
