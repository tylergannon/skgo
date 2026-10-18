package authsample

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
)

// Profile is what a signed-in visitor sees about themselves.
type Profile struct {
	User string `json:"user"`
}

// profile is a query, so it has no event parameter. It reaches the request's
// locals through its context, as Kit's `getRequestEvent()` reaches the event,
// and refuses a caller the handle hook found no session for.
func profile(ctx context.Context) (Profile, error) {
	session := app.LocalsFrom(ctx).Session
	if session.User == "" {
		return Profile{}, skgo.Errorf(http.StatusUnauthorized, "Sign in to see your profile")
	}
	return Profile{User: session.User}, nil
}

var _ = skgo.Query(profile)
