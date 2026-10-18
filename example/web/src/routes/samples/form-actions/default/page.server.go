// Package defaultaction is a page with one unnamed action.
package defaultaction

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
)

// SaveResult is what the page sees as form after a submission.
type SaveResult struct {
	Receipt string `json:"receipt"`
}

// save runs when the page's form is posted without naming an action.
func save(ctx context.Context) (SaveResult, error) {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	return SaveResult{Receipt: "Saved " + request.PostForm.Get("name")}, nil
}

var _ = skgo.DefaultAction(save)
