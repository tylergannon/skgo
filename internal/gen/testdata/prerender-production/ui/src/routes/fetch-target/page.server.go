package fetchtarget

import "github.com/tylergannon/skgo"

type Data struct {
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (Data, error) {
	return Data{Receipt: "literal-fetch-target-data-272"}, nil
}

var _ = skgo.Load(load)
