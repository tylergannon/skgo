package prerender

import (
	"github.com/tylergannon/skgo"
)

type PageData struct {
	Slug    string `json:"slug"`
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	slug := event.Params.Slug()
	return PageData{Slug: slug, Receipt: "Go entry load: " + slug}, nil
}

var _ = skgo.Load(load)
