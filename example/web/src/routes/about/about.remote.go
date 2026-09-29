package about

import (
	"context"

	"github.com/tylergannon/skgo"
)

func buildReceipt(_ context.Context, name string) (string, error) {
	return "Go prerender remote: " + name, nil
}

var _ = skgo.Prerender(buildReceipt)
