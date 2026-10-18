package routing

import "github.com/tylergannon/skgo"

type PageData struct {
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	return PageData{Receipt: "fallback|" + event.Params.Value()}, nil
}

var _ = skgo.Load(load)
