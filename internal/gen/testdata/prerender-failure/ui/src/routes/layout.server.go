package routes

import (
	"errors"

	"github.com/tylergannon/skgo"
)

type Data struct{}

func layoutLoad(LayoutRequestEvent) (Data, error) {
	return Data{}, errors.New("prerender fixture literal failure")
}

var _ = skgo.Load(layoutLoad)
