// Package unexpected is a load that fails the way a bug fails: an ordinary Go
// error, with no status on it.
//
// Kit's rule for one of those is that the visitor is never told what it
// actually said (`runtime/server/errors.js`, `handle_error_and_jsonify`): the
// document is a 500 whose message is whatever the app's own handleError hook
// decides — "Internal Error" with no hook at all, and this app's own words
// with one. Either way the real text below never crosses into the document;
// see example.HandleError.
package unexpected

import (
	"context"
	"errors"

	"github.com/tylergannon/skgo"
)

// PageData is never produced.
type PageData struct {
	// Secret is the value this load would have returned.
	Secret string `json:"secret"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{}, errors.New("the connection string is postgres://ada:hunter2@db")
}

var _ = skgo.Load(pageLoad)
