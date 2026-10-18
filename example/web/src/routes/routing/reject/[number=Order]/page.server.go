package routing

import "github.com/tylergannon/skgo"

type PageData struct {
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	return PageData{Receipt: "accepted-only|" + event.Params.Number().Label()}, nil
}

var _ = skgo.Load(load)
