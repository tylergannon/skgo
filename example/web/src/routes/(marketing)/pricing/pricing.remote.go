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

// Quote is what Go says about a price the browser sent it.
type Quote struct {
	// Heard is the amount Go read back out of the argument, formatted by Go's
	// own method rather than the browser's. If the transport hook were not
	// working this function would never see a Money at all.
	Heard string `json:"heard"`
	// Doubled is that amount doubled, formatted the same way. It is arithmetic
	// only a real Money can do, which is the point: a plain object could not
	// have been doubled.
	Doubled string `json:"doubled"`
}

// quoteFor takes a price from the browser and answers in Go's own words.
func quoteFor(ctx context.Context, price businesslogic.Money) (Quote, error) {
	return Quote{
		Heard:   price.Format(),
		Doubled: businesslogic.Money{Cents: price.Cents * 2}.Format(),
	}, nil
}

var _ = skgo.Command(quoteFor)
