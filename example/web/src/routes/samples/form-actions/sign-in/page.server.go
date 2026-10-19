// Package signin is a page whose actions do not end on it: one redirects and
// one fails on purpose.
package signin

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
)

// signIn remembers the visitor's name and sends them to the home page with a
// 303, so reloading that page does not post the form again.
func signIn(ctx context.Context) error {
	event := skgo.EventFrom(ctx)
	request := event.Request()
	if err := request.ParseForm(); err != nil {
		return skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	username := request.PostForm.Get("username")
	if username == "" {
		return skgo.Errorf(http.StatusBadRequest, "Enter a username")
	}
	if err := event.SetCookie("samples_username", username, skgo.CookieOptions{Path: "/"}); err != nil {
		return err
	}
	return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/"}
}

// forbidden always fails with an expected error: the nearest +error.svelte
// renders with status 403 and this message.
func forbidden(context.Context) error {
	return skgo.Errorf(http.StatusForbidden, "You cannot edit this profile")
}

var _ = skgo.ActionNoData(signIn)
var _ = skgo.ActionNoData(forbidden)
