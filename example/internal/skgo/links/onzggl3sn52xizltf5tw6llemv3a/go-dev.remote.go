package godev

import (
	"context"

	"github.com/tylergannon/skgo"
)

func getRevision(ctx context.Context) (string, error) {
	return "Go wire revision one", nil
}

var _ = skgo.Query(getRevision)
