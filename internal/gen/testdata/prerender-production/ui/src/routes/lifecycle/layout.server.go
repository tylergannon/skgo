package lifecycle

import (
	"fmt"
	"github.com/tylergannon/skgo"
)

type Data struct {
	Layout string `json:"layout"`
}

func load(event LayoutRequestEvent) (Data, error) {
	event.Locals.Calls++
	return Data{Layout: fmt.Sprintf("%s:layout-%d", event.Locals.Value, event.Locals.Calls)}, nil
}

var _ = skgo.Load(load)
