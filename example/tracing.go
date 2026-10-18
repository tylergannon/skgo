package example

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// docs: tracing-setup
// SetupTracing installs a tracer provider and the W3C trace-context
// propagator. Pass sdktrace.WithBatcher(exporter) to send spans somewhere;
// the exporter module is the application's choice.
func SetupTracing(opts ...sdktrace.TracerProviderOption) *sdktrace.TracerProvider {
	provider := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider
}

// docs: end tracing-setup
