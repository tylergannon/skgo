package route

import (
	"context"
	"github.com/tylergannon/skgo"
)

func layout(ctx context.Context) (string, error) {
	_ = skgo.EventFrom(ctx).Param("childPageParameter")
	return "", nil
}

var _ = skgo.Load(layout)
