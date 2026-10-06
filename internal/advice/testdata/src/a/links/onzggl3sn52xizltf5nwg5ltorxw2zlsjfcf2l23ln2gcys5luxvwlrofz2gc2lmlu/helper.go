package route

import (
	"context"
	"github.com/tylergannon/skgo"
)

func load(ctx skgo.RequestEvent[struct{}]) (string, error) {
	e := ctx.Event
	_ = e.Param("customerID")
	_ = e.Param("tab")
	_ = e.Param("tail")
	_ = e.Param("missing") // want `not declared.*empty string.*declared`
	name := "runtime"
	_ = e.Param(name)
	var unrelated *skgo.Event
	_ = unrelated.Param("missing")
	_ = shared(ctx.Context())
	_ = skgo.Load(unregistered)
	return "", nil
}

func shared(ctx context.Context) string { return skgo.EventFrom(ctx).Param("childPageParameter") }

func unregistered(ctx skgo.RequestEvent[struct{}]) (string, error) {
	return ctx.Param("notInRoute"), nil
}
