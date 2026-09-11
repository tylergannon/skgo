// Package expected is a load that refuses on purpose, with a status no other
// function in this app returns.
//
// It is the plain case of kit's error branch: a load throws `error(status,
// message)`, and the document that comes back is the nearest `+error.svelte`
// inside the layouts above it, answered with that status.
package expected

import (
	"context"

	"github.com/tylergannon/skgo"
)

// PageData is never produced.
type PageData struct {
	// Brew would be the drink this page refuses to make.
	Brew string `json:"brew"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{}, skgo.Errorf(418, "This page is a teapot")
}

var _ = skgo.Load(pageLoad)
