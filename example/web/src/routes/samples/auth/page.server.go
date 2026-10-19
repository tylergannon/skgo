// Package authsample is the Auth guide's sample: the page's actions and its
// query each check the session the app's handle hook put in Locals.
package authsample

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/internal/app"
	"github.com/tylergannon/skgo/example/internal/serverhooks"
)

// signIn is a sign-in that works with JavaScript off: a page form action that
// writes the session cookie and redirects. On the next request the handle hook
// reads the cookie and puts the session in Locals.
func signIn(ctx context.Context) error {
	event := skgo.EventFrom(ctx)
	request := event.Request()
	if err := request.ParseForm(); err != nil {
		return skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	user := request.PostForm.Get("username")
	if user == "" {
		return skgo.Errorf(http.StatusBadRequest, "Who are you?")
	}
	id := businesslogic.Default.SignIn(user)
	if err := event.SetCookie(serverhooks.SessionCookie, id, skgo.CookieOptions{MaxAge: 60 * 60}); err != nil {
		return err
	}
	return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/samples/auth"}
}

// publish refuses a caller who is not signed in. The error's status is the
// response's, and Kit renders the nearest +error.svelte.
func publish(ctx context.Context) error {
	if app.LocalsFrom(ctx).Session.User == "" {
		return skgo.Errorf(http.StatusForbidden, "Sign in to publish")
	}
	return nil
}

var (
	_ = skgo.ActionNoData(signIn)
	_ = skgo.ActionNoData(publish)
)
