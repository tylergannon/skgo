// Package item serves the /items/[id] route. It sits in the route directory
// SvelteKit named, brackets and all: the generator gives it an import path Go
// can spell, so nothing about the layout has to bend around Go.
package item

import (
	"context"
	"fmt"

	"github.com/tylergannon/skgo"
)

// Item is one thing in the catalogue.
type Item struct {
	// ID is the value SvelteKit matched into the `[id]` segment.
	ID string `json:"id"`
	// Name is what the page shows.
	Name string `json:"name"`
	// Colocated says where the Go that answered lives, so the page can show
	// that a bracketed directory really is the source of the answer.
	Colocated string `json:"colocated"`
}

func getItem(ctx context.Context, id string) (Item, error) {
	if id == "" {
		return Item{}, skgo.Errorf(400, "An item needs an id")
	}
	return Item{
		ID:        id,
		Name:      fmt.Sprintf("Widget %s", id),
		Colocated: "src/routes/items/[id]/item.remote.go",
	}, nil
}

var _ = skgo.Query(getItem)
