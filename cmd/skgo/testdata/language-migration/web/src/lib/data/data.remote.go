package data

import (
	"context"

	"github.com/tylergannon/skgo"
)

type Receipt struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func receipt(_ context.Context, name string) (Receipt, error) {
	return Receipt{Message: "Go remote: " + name, Count: 7}, nil
}

var _ = skgo.Query(receipt)
