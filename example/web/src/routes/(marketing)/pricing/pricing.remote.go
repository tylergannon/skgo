// Package pricing serves /pricing. Its directory is a SvelteKit layout group,
// `(marketing)`, which never appears in the URL and which Go cannot name — so
// the generator names it for Go.
package pricing

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Plan is one row of the pricing table.
type Plan struct {
	// Name is the plan's name.
	Name string `json:"name"`
	// Price is what it costs per month, in whole dollars.
	Price int `json:"price"`
}

func getPlans(ctx context.Context, _ skgo.None) ([]Plan, error) {
	return []Plan{
		{Name: "Hobby", Price: 0},
		{Name: "Team", Price: 20},
		{Name: "Enterprise", Price: 200},
	}, nil
}

var _ = skgo.Query(getPlans)
