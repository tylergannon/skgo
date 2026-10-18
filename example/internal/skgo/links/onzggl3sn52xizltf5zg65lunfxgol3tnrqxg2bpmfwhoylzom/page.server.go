package routing

import "github.com/tylergannon/skgo"

type PageData struct {
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	return PageData{Receipt: "always|" + event.URL().Path}, nil
}

var _ = skgo.Load(load)
