package skgo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracing mirrors SvelteKit's event.tracing. Root identifies the outermost
// handle span; Current identifies the operation invoking the application.
// Disabled tracing exposes non-recording spans, so annotations are always safe.
// Configure the application's OpenTelemetry provider before serving requests.
type Tracing struct {
	Enabled bool
	Root    trace.Span
	Current trace.Span
}

type tracingKey struct{}
type tracingState struct {
	enabled bool
	root    trace.Span
	native  bool
}

var noSpan = trace.SpanFromContext(context.Background())

func tracingOf(ctx context.Context) Tracing {
	state, _ := ctx.Value(tracingKey{}).(tracingState)
	root := state.root
	if root == nil {
		root = noSpan
	}
	current := noSpan
	if state.enabled {
		current = trace.SpanFromContext(ctx)
	}
	return Tracing{Enabled: state.enabled, Root: root, Current: current}
}

func traceOperation(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	state, _ := ctx.Value(tracingKey{}).(tracingState)
	if !state.enabled || state.native {
		return ctx, noSpan
	}
	return otel.Tracer("sveltekit").Start(ctx, name, trace.WithAttributes(attrs...))
}

// Kit's record_span only marks thrown errors. Response statuses alone are not
// exceptions, and redirects and action failures are expected control flow.
func traceError(span trace.Span, name string, err error) {
	if err == nil {
		return
	}
	var refusal *requestRefusal
	if errors.As(err, &refusal) {
		err = refusal.err
	}
	if redirect := asRedirect(err); redirect != nil {
		span.SetAttributes(attribute.String(name+".result.type", "redirect"), attribute.Int(name+".result.status", redirect.status()), attribute.String(name+".result.location", redirect.Location))
		return
	}
	var failure *actionFailure
	if errors.As(err, &failure) {
		span.SetAttributes(attribute.String(name+".result.type", "failure"), attribute.Int(name+".result.status", failure.status))
		return
	}
	var panicError *tracingPanicError
	if errors.As(err, &panicError) {
		err = fmt.Errorf("%v", panicError.value)
	}
	var known *HTTPError
	if panicError == nil && errors.As(err, &known) {
		span.SetAttributes(attribute.String(name+".result.type", "known_error"), attribute.Int(name+".result.status", known.Status), attribute.String(name+".result.message", known.Message))
		if known.Status < 500 {
			return
		}
		span.AddEvent("exception", trace.WithAttributes(attribute.String("exception.type", "HttpError"), attribute.String("exception.message", known.Message)))
		span.SetStatus(codes.Error, known.Message)
		return
	}
	span.SetAttributes(attribute.String(name+".result.type", "unknown_error"))
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type tracingPanicError struct{ value any }

func (e *tracingPanicError) Error() string { return "Internal Error" }
func (e *tracingPanicError) Unwrap() error { return &HTTPError{Status: 500, Message: "Internal Error"} }
func tracePanic(span trace.Span, name string, value any) {
	if carried, ok := value.(*requestHookPanic); ok {
		value = carried.value
	}
	if carried, ok := value.(*panicWithStack); ok {
		value = carried.value
	}
	if value != nil {
		traceError(span, name, fmt.Errorf("%v", value))
	}
}
func knownRoute(route string) string {
	if route == "" {
		return "unknown"
	}
	return route
}
func traceRoute(ctx context.Context) string {
	if event := EventFrom(ctx); event != nil {
		if metadata := event.metadata; metadata != nil && metadata.routeID != "" {
			return metadata.routeID
		}
		if event.load != nil && event.load.shared.routeID != "" {
			return event.load.shared.routeID
		}
	}
	return "unknown"
}
func traceSequence(ctx context.Context, name string) (context.Context, trace.Span) {
	state, _ := ctx.Value(tracingKey{}).(tracingState)
	if !state.enabled {
		return ctx, noSpan
	}
	return otel.Tracer("sveltekit").Start(ctx, name)
}
func traceHookName(fn any, index int) string {
	name := ""
	if function := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()); function != nil {
		name = function.Name()
		if strings.Contains(name, ".func") {
			return "sveltekit.handle.sequenced." + fmt.Sprint(index)
		}
		name = name[strings.LastIndex(name, ".")+1:]
		// Compiler-generated closures are anonymous, like Kit's numbered handlers.
		if strings.HasPrefix(name, "func") || strings.Contains(name, "[") {
			name = ""
		}
	}
	if name == "" {
		name = fmt.Sprint(index)
	}
	return "sveltekit.handle.sequenced." + name
}

// An HTTP handler constructs its Response when it commits headers. Kit's spans
// end at Response construction, before a potentially unbounded body is consumed.
type tracingWriter struct {
	http.ResponseWriter
	once   sync.Once
	finish func()
}

func (w *tracingWriter) end()                        { w.once.Do(w.finish) }
func (w *tracingWriter) WriteHeader(status int)      { w.end(); w.ResponseWriter.WriteHeader(status) }
func (w *tracingWriter) Write(p []byte) (int, error) { w.end(); return w.ResponseWriter.Write(p) }
func (w *tracingWriter) Flush()                      { w.end(); _ = http.NewResponseController(w.ResponseWriter).Flush() }
func (w *tracingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

var w3cTrace = propagation.TraceContext{}

func remoteTraceKind(kind Kind) string {
	switch kind {
	case KindQuery:
		return "query"
	case KindBatch:
		return "query_batch"
	case KindLive:
		return "query_live"
	case KindCommand:
		return "command"
	case KindForm:
		return "form"
	case KindPrerender:
		return "prerender"
	}
	return "unknown"
}

func endSequenceSpan(span trace.Span, name string) {
	if value := recover(); value != nil {
		tracePanic(span, name, value)
		span.End()
		panic(value)
	}
	span.End()
}
