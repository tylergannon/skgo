// Package asyncssr demonstrates independent I/O during a page render.
package asyncssr

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

type Input struct {
	Group string `json:"group"`
	Key   string `json:"key"`
}

type gate struct {
	mu      sync.Mutex
	started map[string]bool
	ready   chan struct{}
	users   int
}

var gates sync.Map

// Marks a group whose three-way overlap has already been proven once. A
// live query's browser-side reconnect calls back into the same group alone
// — ordinary and batch queries are cached across hydration and never refire,
// so it has no other participants to overlap with. That reconnect is a
// legitimate continuation of the round the document already rendered, not a
// second overlap to prove, so once a group has settled, later callers
// return their fixture value immediately instead of waiting for partners
// that were never going to arrive.
var settled sync.Map

// A fixture, not a latency benchmark: no operation can return a value until
// all three distinct operations enter. Blocking dispatch therefore fails.
func overlap(ctx context.Context, group, key string) (string, error) {
	values := map[string]string{"amber": "Amber", "birch": "Birch", "cobalt": "Cobalt", "batch": "Birch", "live": "Cobalt"}
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("unknown fixture key %q", key)
	}

	if _, done := settled.Load(group); done {
		return value, nil
	}

	candidate := &gate{started: map[string]bool{}, ready: make(chan struct{})}
	stored, _ := gates.LoadOrStore(group, candidate)
	g := stored.(*gate)
	g.mu.Lock()
	if !g.started[key] {
		g.started[key] = true
		if len(g.started) == 3 {
			close(g.ready)
		}
	}
	g.users++
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.users--
		if g.users == 0 {
			gates.CompareAndDelete(group, g)
		}
	}()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timeout.C:
		return "", skgo.Errorf(504, "Independent operations did not overlap")
	case <-g.ready:
	}
	settled.Store(group, struct{}{})
	return value, nil
}

func getValue(ctx context.Context, input Input) (string, error) {
	return overlap(ctx, input.Group, input.Key)
}

func getBatch(ctx context.Context, inputs []Input) ([]string, error) {
	if len(inputs) != 2 || inputs[0].Group != inputs[1].Group {
		return nil, fmt.Errorf("expected both batch arguments together")
	}
	value, err := overlap(ctx, inputs[0].Group, "batch")
	if err != nil {
		return nil, err
	}
	return []string{value + " one", value + " two"}, nil
}

func watchValue(ctx context.Context, input Input, yield func(string) error) error {
	value, err := overlap(ctx, input.Group, "live")
	if err != nil {
		return err
	}
	return yield(value)
}

func getSeed(ctx context.Context, group string) (string, error) { return "seed:" + group, nil }
func getDependent(ctx context.Context, seed string) (string, error) {
	if !strings.HasPrefix(seed, "seed:") {
		return "", fmt.Errorf("dependent query did not receive its input")
	}
	return "Harvest from " + seed, nil
}

var _ = skgo.Query(getValue)
var _ = skgo.BatchQuery(getBatch)
var _ = skgo.LiveQuery(watchValue)
var _ = skgo.Query(getSeed)
var _ = skgo.Query(getDependent)

func getHeld(ctx context.Context, input Input) (string, error) {
	return businesslogic.RenderProbe(input.Group).Value(ctx, input.Group, input.Key)
}

var _ = skgo.Query(getHeld)
