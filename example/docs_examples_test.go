//go:build skgo_qualification

package example_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestAnOptionalParameterIsNilWhenTheSegmentIsAbsent(t *testing.T) {
	h := newProdHandler(t)
	for path, want := range map[string]string{"/lang/__data.json": `"en"`, "/lang/fr/__data.json": `"fr"`} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s: %d, want a body carrying %s\n%s", path, rec.Code, want, rec.Body.String())
		}
	}
}

// TestATracedLoadRecordsItsSpanUnderTheFrameworkRoot installs a provider with
// a span recorder and walks from the span the load opened up to the root span
// the framework opened for the request.
func TestATracedLoadRecordsItsSpanUnderTheFrameworkRoot(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	previous, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	provider := example.SetupTracing(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})
	if _, ok := otel.GetTextMapPropagator().(propagation.TraceContext); !ok {
		t.Fatalf("SetupTracing installed %T, want the W3C trace-context propagator", otel.GetTextMapPropagator())
	}

	rec := get(t, newProdHandler(t), "/traced/__data.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /traced/__data.json: %d\n%s", rec.Code, rec.Body.String())
	}

	parents := map[string]string{}
	names := map[string]string{}
	var work sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		id := span.SpanContext().SpanID().String()
		parents[id], names[id] = span.Parent().SpanID().String(), span.Name()
		if span.Name() == "report.prepare" {
			work = span
		}
	}
	if work == nil {
		t.Fatalf("the load's span was not recorded; got %v", names)
	}
	var chain []string
	for id := work.SpanContext().SpanID().String(); id != "" && names[id] != ""; id = parents[id] {
		chain = append(chain, names[id])
	}
	if last := chain[len(chain)-1]; last != "sveltekit.handle.root" {
		t.Fatalf("the load's span descends from %v, want the sveltekit.handle.root span", chain)
	}
	for _, span := range recorder.Ended() {
		if span.Name() == "sveltekit.handle.root" && !hasAttribute(span, "user.id", "42") {
			t.Errorf("the root span lacks the user.id attribute the load set: %v", span.Attributes())
		}
	}
}

func hasAttribute(span sdktrace.ReadOnlySpan, key, value string) bool {
	for _, a := range span.Attributes() {
		if string(a.Key) == key && a.Value.AsString() == value {
			return true
		}
	}
	return false
}
