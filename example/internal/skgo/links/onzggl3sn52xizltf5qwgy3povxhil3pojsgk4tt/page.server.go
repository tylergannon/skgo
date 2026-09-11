// Package orders is the second page under the /account layout. It has no guard
// of its own: the layout's rule already covers it.
package orders

import (
	"context"
	"time"

	"github.com/tylergannon/skgo"
)

// Order is one line of the visitor's order history.
type Order struct {
	// Item is what was bought.
	Item string `json:"item"`
	// Cents is what it cost.
	Cents int `json:"cents"`
}

// PageData is the orders page. The total is known at once; the orders
// themselves are slow, so they are deferred and arrive on the same response.
type PageData struct {
	// OrderTotal is on screen immediately.
	OrderTotal int `json:"orderTotal"`
	// Orders arrives later, without a second request.
	Orders skgo.Deferred[[]Order] `json:"orders"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{
		OrderTotal: 2,
		Orders: skgo.Async(ctx, func(ctx context.Context) ([]Order, error) {
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []Order{
				{Item: "a slow parcel", Cents: 1999},
				{Item: "a slower parcel", Cents: 2999},
			}, nil
		}),
	}, nil
}

var _ = skgo.Load(pageLoad)
