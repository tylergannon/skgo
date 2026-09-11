// Package statement is the page whose load fails, so that a visitor can see
// what a failed load does: an error page with the status and message the Go
// function chose, inside the layout that declared the error page.
package statement

import (
	"context"

	"github.com/tylergannon/skgo"
)

// PageData is never produced.
type PageData struct {
	// Balance would be the amount owed.
	Balance int `json:"balance"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{}, skgo.Errorf(402, "Your account is in arrears")
}

var _ = skgo.Load(pageLoad)
