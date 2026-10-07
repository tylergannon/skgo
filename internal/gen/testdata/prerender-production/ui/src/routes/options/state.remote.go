package options

import (
	"context"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
)

func ready(ctx context.Context) (string, error) {
	app.LocalsFrom(ctx).HeadersReady = true
	return "headers-now-ready", nil
}

var _ = skgo.Prerender(ready)
