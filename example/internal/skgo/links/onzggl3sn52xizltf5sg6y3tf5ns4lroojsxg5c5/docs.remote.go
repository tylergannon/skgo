// Package docs serves /docs and everything under it. `[...rest]` is
// SvelteKit's rest parameter; as a Go directory name it is doubly impossible,
// which is exactly why the generator addresses it through a link.
package docs

import (
	"context"
	"strings"

	"github.com/tylergannon/skgo"
)

// Page is one documentation page.
type Page struct {
	// Path is the rest parameter as SvelteKit matched it.
	Path string `json:"path"`
	// Title is what the page shows.
	Title string `json:"title"`
	// Depth is how many segments deep the request went.
	Depth int `json:"depth"`
}

func getPage(ctx context.Context, path string) (Page, error) {
	segments := []string{}
	for _, segment := range strings.Split(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	title := "Docs"
	if len(segments) > 0 {
		title = strings.Join(segments, " / ")
	}
	return Page{Path: path, Title: title, Depth: len(segments)}, nil
}

var _ = skgo.Query(getPage)
