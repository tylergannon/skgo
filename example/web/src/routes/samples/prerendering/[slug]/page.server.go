package prerendering

import "github.com/tylergannon/skgo"

type PageData struct {
	Slug string `json:"slug"`
}

// load runs once for each entry +page.ts lists, while Kit builds the app.
func load(event PageRequestEvent) (PageData, error) {
	return PageData{Slug: event.Params.Slug()}, nil
}

var _ = skgo.Load(load)
