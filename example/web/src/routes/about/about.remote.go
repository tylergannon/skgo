package about

import (
	"context"

	"github.com/tylergannon/skgo"
)

func buildReceipt(_ context.Context, name string) (string, error) {
	return "Go prerender remote: " + name, nil
}

func receiptNames() ([]string, error) {
	return []string{"atlas", "beacon"}, nil
}

var _ = skgo.Prerender(buildReceipt, skgo.PrerenderOptions{Inputs: receiptNames})
