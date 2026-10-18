package remotefunctions

import (
	"context"

	"github.com/tylergannon/skgo"
)

// watchCount pushes how many notes this visitor has, now and after every change,
// until the client disconnects and ctx is cancelled.
func watchCount(ctx context.Context, yield func(int) error) error {
	for {
		lists.Lock()
		l := mine(ctx)
		count, changed := len(l.notes), l.changed
		lists.Unlock()

		if err := yield(count); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-changed:
		}
	}
}

var _ = skgo.LiveQuery(watchCount)
