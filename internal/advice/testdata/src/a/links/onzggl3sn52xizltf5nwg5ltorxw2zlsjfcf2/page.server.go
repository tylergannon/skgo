package route

import (
	"context"
	"github.com/tylergannon/skgo"
)

func load(ctx context.Context) (struct{}, error) {
	_ = skgo.EventFrom(ctx).Param("id") // want `not declared.*empty string.*declared`
	_ = skgo.EventFrom(ctx).Param("customerID")
	param := "dynamic"
	_ = skgo.EventFrom(ctx).Param(param)
	var unrelated *skgo.Event
	_ = unrelated.Param("absent") // origin is not the load's event
	_ = shared(ctx)
	return struct{}{}, nil
}

func shared(ctx context.Context) string { return skgo.EventFrom(ctx).Param("absent") }

var _ = skgo.Load(load)
