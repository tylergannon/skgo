// Package pricing serves /pricing. Its directory is a SvelteKit layout group,
// `(marketing)`, which never appears in the URL and which Go cannot name — so
// the generator names it for Go.
package pricing

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// Plan is one row of the pricing table.
type Plan struct {
	// Name is the plan's name.
	Name string `json:"name"`
	// Price is what it costs per month. It is a domain type with a method,
	// and it reaches the browser as one because the app declares a `transport`
	// hook for it in src/hooks.go and src/hooks.ts.
	Price businesslogic.Money `json:"price"`
}

func getPlans(ctx context.Context, _ skgo.None) ([]Plan, error) {
	return []Plan{
		{Name: "Hobby", Price: businesslogic.USD(0)},
		{Name: "Team", Price: businesslogic.USD(20)},
		{Name: "Enterprise", Price: businesslogic.USD(200)},
	}, nil
}

var _ = skgo.Query(getPlans)
