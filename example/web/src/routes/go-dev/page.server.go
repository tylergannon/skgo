package godev

import (
	"context"

	"github.com/tylergannon/skgo"
)

// PageData is what the page is handed. The browser proof for development
// updates edits this file and the remote beside it while the server runs.
type PageData struct {
	Revision string `json:"revision"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{Revision: "Go revision one"}, nil
}

// ActionResult is what the page's `revise` action answers a form with.
type ActionResult struct {
	Message string `json:"message"`
}

func revise(ctx context.Context) (ActionResult, error) {
	return ActionResult{Message: "Go action revision one"}, nil
}

var _ = skgo.Load(pageLoad)
var _ = skgo.Action(revise)
