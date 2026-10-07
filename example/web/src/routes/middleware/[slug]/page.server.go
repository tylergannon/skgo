// Package middlewareslug serves /middleware/[slug], reached from /middleware by
// a client navigation, which asks for its data over `__data.json`.
package middlewareslug

import (
	"net/http"

	app "github.com/tylergannon/skgo/example/internal/app"

	"github.com/tylergannon/skgo"
)

// PageData is what the middleware observed on the request event.
type PageData struct {
	Token string `json:"token"`
	Route string `json:"route"`
	Slug  string `json:"slug"`
	Data  bool   `json:"data"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	visit := app.LocalsFrom(ctx).Visit
	ok := visit.Token != ""
	if !ok {
		return PageData{}, skgo.Errorf(http.StatusInternalServerError, "the middleware established no visit")
	}
	return PageData{Token: visit.Token, Route: visit.Route, Slug: visit.Slug, Data: visit.Data}, nil
}

var _ = skgo.Load(pageLoad)
