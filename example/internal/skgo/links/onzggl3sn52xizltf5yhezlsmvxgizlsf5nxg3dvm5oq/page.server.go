package prerender

import (
	"context"

	"github.com/tylergannon/skgo"
)

type PageData struct {
	Slug    string `json:"slug"`
	Receipt string `json:"receipt"`
}

func load(ctx context.Context) (PageData, error) {
	slug := skgo.EventFrom(ctx).Param("slug")
	return PageData{Slug: slug, Receipt: "Go entry load: " + slug}, nil
}

var _ = skgo.Load(load)
