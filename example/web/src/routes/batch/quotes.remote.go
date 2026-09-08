// Package batch serves the /batch route: several components each asking for one
// value, answered by one call to one Go function.
package batch

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Quote is one symbol's price.
type Quote struct {
	// Symbol is the symbol that was asked for.
	Symbol string `json:"symbol"`
	// Cents is what it costs.
	Cents int `json:"cents"`
	// BatchSize is how many symbols the Go function was handed in the call
	// that answered this one.
	//
	// It is what makes batching visible rather than implied. Four components
	// each asking for one symbol should each be answered "4": one call, four
	// arguments. If every call carried a single argument the page would say
	// "1" four times, which is a page that works and a feature that does not.
	BatchSize int `json:"batchSize"`
}

// prices is the whole catalogue. Every number here appears nowhere else in the
// app, so a price on the page has exactly one source.
var prices = map[string]int{
	"SKGO": 1275,
	"GOJA": 3460,
	"KITX": 5610,
	"SVLT": 7845,
}

// getQuotes answers a whole batch of symbols in one call.
//
// The page still asks for one symbol at a time — `getQuotes('SKGO')` — and kit
// collects every such call made in the same turn into this one invocation,
// during a render exactly as in the browser. One result per symbol, in the same
// order: that is how kit's client matches answers to the promises it is
// holding.
func getQuotes(ctx context.Context, symbols []string) ([]Quote, error) {
	out := make([]Quote, len(symbols))
	for i, symbol := range symbols {
		cents, ok := prices[symbol]
		if !ok {
			return nil, skgo.Errorf(404, "No quote for %q", symbol)
		}
		out[i] = Quote{Symbol: symbol, Cents: cents, BatchSize: len(symbols)}
	}
	return out, nil
}

var _ = skgo.BatchQuery(getQuotes)
