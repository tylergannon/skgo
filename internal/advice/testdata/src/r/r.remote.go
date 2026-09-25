package r

import (
	"context"
	"github.com/tylergannon/skgo"
)

func query(ctx context.Context, id string) (string, error)                { return id, nil }
func noArg(ctx context.Context) (string, error)                           { return "", nil }
func command(ctx context.Context, id string) (string, error)              { return id, nil }
func noArgCommand(ctx context.Context) (string, error)                    { return "", nil }
func batch(ctx context.Context, ids []string) ([]string, error)           { return ids, nil }
func live(ctx context.Context, id string, yield func(string) error) error { return yield(id) }
func noArgLive(ctx context.Context, yield func(string) error) error       { return yield("") }
func liveShapedCommand(ctx context.Context, id string, yield func(string) error) error {
	return yield(id)
}
func noArgLiveShapedCommand(ctx context.Context, yield func(string) error) error { return yield("") }
func unknown(ctx context.Context, id string) (string, error)                     { return id, nil }

var _ = skgo.Query(query)
var _ = skgo.Query(noArg)
var _ = skgo.Command(command)
var _ = skgo.Command(noArgCommand)
var _ = skgo.BatchQuery(batch)
var _ = skgo.LiveQuery(live)
var _ = skgo.LiveQuery(noArgLive)
var _ = skgo.Command(liveShapedCommand)
var _ = skgo.Command(noArgLiveShapedCommand)

func run(ctx context.Context) (string, error) {
	if err := skgo.Refresh(ctx, command, "x"); err != nil { // want `registered Command.*rejects.*registered Query`
		return "", err
	}
	if err := skgo.RefreshRequested(ctx, command, 1); err != nil { // want `registered Command.*rejects.*registered Query`
		return "", err
	}
	if err := skgo.RefreshNoArg(ctx, noArgCommand); err != nil { // want `registered Command.*rejects.*registered Query`
		return "", err
	}
	if err := skgo.Refresh(ctx, batch, []string{"x"}); err != nil { // want `registered BatchQuery.*rejects.*registered Query`
		return "", err
	}
	if err := skgo.ReconnectRequested(ctx, liveShapedCommand, 1); err != nil { // want `registered Command.*rejects.*registered LiveQuery`
		return "", err
	}
	if err := skgo.ReconnectRequestedNoArg(ctx, noArgLiveShapedCommand); err != nil { // want `registered Command.*rejects.*registered LiveQuery`
		return "", err
	}
	if err := skgo.Refresh(ctx, query, "x"); err != nil {
		return "", err
	}
	if err := skgo.RefreshRequested(ctx, query, 1); err != nil {
		return "", err
	}
	if err := skgo.RefreshNoArg(ctx, noArg); err != nil {
		return "", err
	}
	if err := skgo.ReconnectRequested(ctx, live, 1); err != nil {
		return "", err
	}
	if err := skgo.ReconnectRequestedNoArg(ctx, noArgLive); err != nil {
		return "", err
	}
	if err := skgo.Refresh(ctx, unknown, "x"); err != nil {
		return "", err
	}
	return "", nil
}

var _ = skgo.Command(run)

// Calling a marker at runtime does not create a generator registration.
func misleading(ctx context.Context) (string, error) {
	_ = skgo.Command(unknown)
	return "", skgo.Refresh(ctx, unknown, "x")
}

var _ = skgo.Command(misleading)
