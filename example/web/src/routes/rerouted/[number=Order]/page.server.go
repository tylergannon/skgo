package rerouted

import "github.com/tylergannon/skgo"

type PageData struct {
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	return PageData{Receipt: "page|route=" + event.RouteID() + "|param=" + event.Params.Number().Label() + "|url=" + event.URL().RequestURI()}, nil
}

var _ = skgo.Load(load)
