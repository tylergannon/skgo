// Package slow ports the streamed half of the junkyard app's /slow load.
package slow

import (
	"context"
	"time"

	"github.com/tylergannon/skgo"
)

// getSlowPart takes a second and a half, the same as the promise the junkyard
// load returned unawaited.
func getSlowPart(ctx context.Context, _ skgo.None) (string, error) {
	select {
	case <-time.After(1500 * time.Millisecond):
		return "slow part", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

var _ = skgo.Query(getSlowPart)
