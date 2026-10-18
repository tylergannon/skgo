package skgo_test

import (
	"context"
	"fmt"

	"github.com/tylergannon/skgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Tracing is Kit's event.tracing: Enabled reports whether server tracing is
// on, Root is the sveltekit.handle.root span and Current is the span of the
// load, action, endpoint or remote call running your code. An application
// reads it as skgo.EventFrom(ctx).Tracing, or event.Tracing on a typed event.
// With tracing off both spans are non-recording, so annotations still run and
// do nothing.
func ExampleTracing() {
	annotate := func(tracing skgo.Tracing) {
		tracing.Root.SetAttributes(attribute.String("user.id", "42"))
		tracing.Current.SetAttributes(attribute.String("report.kind", "monthly"))
	}

	off := trace.SpanFromContext(context.Background())
	tracing := skgo.Tracing{Enabled: false, Root: off, Current: off}
	annotate(tracing)
	fmt.Println(tracing.Enabled, tracing.Current.IsRecording())
	// Output: false false
}
