// Package stream is the page that shows what a promised value does.
//
// Its load hands back three values it does not have yet, at three different
// depths, and each one takes a different amount of time to arrive. The page
// leaves Go with three loading states already rendered into it and fills them
// in one at a time, on the same response, in the order the work finishes —
// which is deliberately not the order kit numbered them in.
package stream

import (
	"context"
	"time"

	"github.com/tylergannon/skgo"
)

// The three waits. They are far enough apart that the page is legibly half
// finished in between, and they are ordered against the alphabet: kit numbers
// promises in the order devalue walks the load's result, and devalue sorts an
// object's keys, so "digest" is numbered before "ticker" and settles after it.
const (
	tickerWait   = 800 * time.Millisecond
	digestWait   = 2200 * time.Millisecond
	forecastWait = 3600 * time.Millisecond
)

// Panel is one section of the page. It exists to put a promised value somewhere
// other than the top level of the load's result, which is where kit allows one:
// its serializer finds a promise with a devalue reducer and its client reads one
// back with a devalue reviver, and both walk the whole tree.
type Panel struct {
	// Heading is on screen at once.
	Heading string `json:"heading"`
	// Forecast is the slowest of the three, and the one that is not a field of
	// what the load returned.
	Forecast skgo.Deferred[string] `json:"forecast"`
}

// PageData is the streaming page.
type PageData struct {
	// Headline is the value the load had in hand.
	Headline string `json:"headline"`
	// Digest is numbered first and arrives second.
	Digest skgo.Deferred[[]string] `json:"digest"`
	// Ticker is numbered second and arrives first.
	Ticker skgo.Deferred[string] `json:"ticker"`
	// Weather holds a promise below the top level.
	Weather Panel `json:"weather"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{
		Headline: "Three promises, one response",
		Digest: after(ctx, digestWait, []string{
			"the second thing to arrive",
			"and the rest of the digest",
		}),
		Ticker:  after(ctx, tickerWait, "the first thing to arrive"),
		Weather: Panel{Heading: "Forecast", Forecast: after(ctx, forecastWait, "the last thing to arrive")},
	}, nil
}

// after is the load's whole implementation: work that takes a while and then
// answers. A visitor who closes the tab cancels it.
func after[T any](ctx context.Context, wait time.Duration, value T) skgo.Deferred[T] {
	return skgo.Async(ctx, func(ctx context.Context) (T, error) {
		select {
		case <-time.After(wait):
			return value, nil
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	})
}

var _ = skgo.Load(pageLoad)
