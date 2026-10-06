package typeddependencies

import "github.com/tylergannon/skgo"

type PageData struct {
	Label string `json:"label"`
}

func load(event RequestEvent) (PageData, error) {
	read, _ := event.SearchParam("read")
	if read == "b" {
		return PageData{Label: event.Params.B().Label()}, nil
	}
	return PageData{Label: event.Params.A().Label()}, nil
}

var _ = skgo.Load(load)
