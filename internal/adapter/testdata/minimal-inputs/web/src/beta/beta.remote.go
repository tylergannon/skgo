package beta

import (
	"context"
	"github.com/tylergannon/skgo"
)

func item(_ context.Context, value int) (int, error) { return value + 20, nil }
func names() ([]int, error)                          { return []int{5}, nil }

var _ = skgo.Prerender(item, skgo.PrerenderOptions{Inputs: names})
