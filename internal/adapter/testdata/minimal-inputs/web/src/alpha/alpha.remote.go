package alpha

import (
	"context"
	"github.com/tylergannon/skgo"
)

func item(_ context.Context, value string) (string, error) { return "alpha:" + value, nil }
func names() ([]string, error)                             { return []string{"atlas"}, nil }

var _ = skgo.Prerender(item, skgo.PrerenderOptions{Inputs: names})
