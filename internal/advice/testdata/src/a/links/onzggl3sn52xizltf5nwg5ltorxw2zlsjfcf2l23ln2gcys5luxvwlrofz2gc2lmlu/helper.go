package route

import (
	"context"
	"github.com/tylergannon/skgo"
)

func load(ctx context.Context) (string, error) {
	e := skgo.EventFrom(ctx)
	_ = e.Param("customerID")
	_ = e.Param("tab")
	_ = e.Param("tail")
	_ = e.Param("missing") // want `not declared.*empty string.*declared`
	name := "runtime"
	_ = e.Param(name)
	var unrelated *skgo.Event
	_ = unrelated.Param("missing")
	_ = shared(ctx)
	_ = skgo.Load(unregistered)
	return "", nil
}

func shared(ctx context.Context) string { return skgo.EventFrom(ctx).Param("childPageParameter") }

func unregistered(ctx context.Context) (string, error) {
	return skgo.EventFrom(ctx).Param("notInRoute"), nil
}
