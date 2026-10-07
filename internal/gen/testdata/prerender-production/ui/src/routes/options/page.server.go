package options

import "github.com/tylergannon/skgo"

type Data struct {
	Ready string `json:"ready"`
}

func load(event PageRequestEvent) (Data, error) {
	event.Locals.AssetsReady = true
	return Data{Ready: "post-load-state"}, nil
}

var _ = skgo.Load(load)
