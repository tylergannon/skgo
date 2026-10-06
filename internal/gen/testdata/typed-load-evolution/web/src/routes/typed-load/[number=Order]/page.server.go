package typedload

import "github.com/tylergannon/skgo"

type PageData struct {
	Label string `json:"label"`
}

func load(event RequestEvent) (PageData, error) {
	return PageData{Label: event.Params.Number().Label()}, nil
}

var _ = skgo.Load(load)
