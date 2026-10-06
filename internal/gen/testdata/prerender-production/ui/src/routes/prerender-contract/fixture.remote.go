package prerendercontract

import (
	"context"
	"sync/atomic"

	"github.com/tylergannon/skgo"
)

var calls atomic.Int32

func noargValue(context.Context) (string, error) {
	calls.Add(1)
	return "live no-argument body ran", nil
}

func Calls() int32 { return calls.Load() }

var _ = skgo.Prerender(noargValue)
