package routing

import "github.com/tylergannon/skgo"

type PageData struct {
	Receipt string `json:"receipt"`
}

func load(event PageRequestEvent) (PageData, error) {
	value := "omitted"
	if n := event.Params.Number(); n != nil {
		value = n.Label()
	}
	return PageData{Receipt: "optional|" + value}, nil
}

var _ = skgo.Load(load)
