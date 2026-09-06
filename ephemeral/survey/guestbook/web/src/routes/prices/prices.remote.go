// Package prices ports the junkyard app's custom-transport page: a `Money`
// class instance crossing the wire in both a load and a query.
package prices

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Money is the Go side of the app's `Money` class. The junkyard app declared a
// `transport` codec so that both sides see a real `Money`, not a plain object.
type Money struct {
	Cents    int    `json:"cents"`
	Currency string `json:"currency"`
}

func getListPrice(ctx context.Context, _ skgo.None) (Money, error) {
	return Money{Cents: 1999, Currency: "USD"}, nil
}

func getSalePrice(ctx context.Context, _ skgo.None) (Money, error) {
	return Money{Cents: 1499, Currency: "USD"}, nil
}

var (
	_ = skgo.Query(getListPrice)
	_ = skgo.Query(getSalePrice)
)
