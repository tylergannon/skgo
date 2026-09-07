// Package boundary holds a query that refuses, for a page that catches the
// refusal itself.
//
// It is there to pin a rule of kit's that surprises people: `transformError` is
// installed once for the whole render (`page/render.js`) and Svelte calls it
// for *every* boundary with a `failed` snippet, including the app's own. So a
// component that handles its own failure gracefully still moves `page.status`
// and `page.error` for the whole document, and the response carries the status
// the caught error had.
package boundary

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Reading is the value this query never produces.
type Reading struct {
	// Celsius is the temperature that would have been read.
	Celsius int `json:"celsius"`
}

func readSensor(_ context.Context) (Reading, error) {
	return Reading{}, skgo.Errorf(409, "The sensor is being calibrated")
}

var _ = skgo.Query(readSensor)
