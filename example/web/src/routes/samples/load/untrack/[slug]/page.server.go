// Package slug reads its route parameter without depending on it.
package slug

import (
	"github.com/tylergannon/skgo"
)

// PageData is the slug the load first saw.
type PageData struct {
	Slug string `json:"slug"`
}

// pageLoad reads the slug inside Untrack, so navigating to another slug does
// not run it again.
func pageLoad(event PageRequestEvent) (PageData, error) {
	var slug string
	event.Untrack(func() { slug = event.Params.Slug() })
	return PageData{Slug: slug}, nil
}

var _ = skgo.Load(pageLoad)
