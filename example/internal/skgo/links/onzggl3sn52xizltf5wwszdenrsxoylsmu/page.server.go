// Package middlewarepage serves /middleware, the page the example's Go
// middleware authenticates before anything on it answers.
package middlewarepage

import (
	"context"
	"net/http"

	app "github.com/tylergannon/skgo/example/internal/app"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// PageData is what the load read of the visit the middleware established, and
// the cookie the load itself saw in the request's jar.
type PageData struct {
	Token  string `json:"token"`
	Route  string `json:"route"`
	Cookie string `json:"cookie"`
}

func pageLoad(event RequestEvent) (PageData, error) {
	ctx := event.Context()
	visit := app.LocalsFrom(ctx).Visit
	ok := visit.Token != ""
	if !ok {
		return PageData{}, skgo.Errorf(http.StatusInternalServerError, "the middleware established no visit")
	}
	cookie, _ := event.Cookie(businesslogic.VisitCookie)
	return PageData{Token: visit.Token, Route: visit.Route, Cookie: cookie}, nil
}

var _ = skgo.Load(pageLoad)

// NoteResult is the typed answer to the form.
type NoteResult struct {
	Receipt string `json:"receipt"`
}

func note(ctx context.Context) (NoteResult, error) {
	visit := app.LocalsFrom(ctx).Visit
	ok := visit.Token != ""
	if !ok {
		return NoteResult{}, skgo.Errorf(http.StatusInternalServerError, "the middleware established no visit")
	}
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseForm(); err != nil {
		return NoteResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	return NoteResult{Receipt: "Noted " + r.PostForm.Get("note") + " for " + visit.Token}, nil
}

var _ = skgo.DefaultAction(note)
