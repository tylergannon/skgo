// Package traced is a load and a remote function that annotate the spans the
// framework opens for them.
package traced

import (
	"github.com/tylergannon/skgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type PageData struct {
	TraceID string `json:"traceId"`
}

// docs: traced-load
func pageLoad(event PageRequestEvent) (PageData, error) {
	event.Tracing.Root.SetAttributes(attribute.String("user.id", "42"))
	event.Tracing.Current.SetAttributes(attribute.String("report.kind", "monthly"))

	_, span := otel.Tracer("myapp").Start(event.Context(), "report.prepare")
	defer span.End()

	return PageData{TraceID: event.Tracing.Current.SpanContext().TraceID().String()}, nil
}

var _ = skgo.Load(pageLoad)

// docs: end traced-load
