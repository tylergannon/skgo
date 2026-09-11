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

func getPlans(ctx context.Context) ([]Plan, error) {
	return []Plan{
		{Name: "Hobby", Price: businesslogic.USD(0)},
		{Name: "Team", Price: businesslogic.USD(20)},
		{Name: "Enterprise", Price: businesslogic.USD(200)},
	}, nil
}

var _ = skgo.Query(getPlans)

// getSpotlight is the plan the page leads with, and it is the fixture for a
// transported type in a *remote* answer during a render.
//
// The plans list below sits in a boundary with a `pending` snippet, and Svelte's
// server compiler emits that snippet instead of the boundary's children, so
// `getPlans` is never called while the document is being built. This one is
// awaited in a boundary with no pending snippet: the engine calls back into Go
// while the page renders, Go answers with a businesslogic.Money under the
// transport key, and the app's own decoder rebuilds it before `format()` runs.
// The price it writes is therefore in the bytes Go sent.
//
// 750 cents is an amount no other function in this app returns, so nothing on
// the page can show "$7.50" by accident.
func getSpotlight(ctx context.Context) (Plan, error) {
	return Plan{Name: "Student", Price: businesslogic.Money{Cents: 750}}, nil
}

var _ = skgo.Query(getSpotlight)

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
