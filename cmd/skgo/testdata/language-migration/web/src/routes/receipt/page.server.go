package receipt

import (
	"context"

	"github.com/tylergannon/skgo"
)

type PageData struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type Saved struct {
	Message string `json:"message"`
}

func page(PageRequestEvent) (PageData, error) {
	return PageData{Message: "Go load: migration", Count: 11}, nil
}

func save(context.Context) (Saved, error) {
	return Saved{Message: "Go action: saved"}, nil
}

var (
	_ = skgo.Load(page)
	_ = skgo.Action(save)
)
