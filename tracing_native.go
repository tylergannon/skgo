package skgo

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Native Kit owns these spans. Go holds their standard remote contexts and
// returns annotations through the existing private callback response.
type nativeSpanInfo struct {
	TraceID    string `json:"traceId"`
	SpanID     string `json:"spanId"`
	TraceFlags int    `json:"traceFlags"`
	TraceState string `json:"traceState,omitempty"`
	Recording  bool   `json:"recording"`
}

func nativeSpanContext(info nativeSpanInfo) trace.SpanContext {
	tid, _ := trace.TraceIDFromHex(info.TraceID)
	sid, _ := trace.SpanIDFromHex(info.SpanID)
	ts, _ := trace.ParseTraceState(info.TraceState)
	return trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: trace.TraceFlags(info.TraceFlags), TraceState: ts, Remote: true})
}
func nativeInfo(span trace.Span) nativeSpanInfo {
	sc := span.SpanContext()
	return nativeSpanInfo{TraceID: sc.TraceID().String(), SpanID: sc.SpanID().String(), TraceFlags: int(sc.TraceFlags()), TraceState: sc.TraceState().String(), Recording: span.IsRecording()}
}

type nativeTraceInput struct {
	Enabled     bool              `json:"enabled"`
	Propagation map[string]string `json:"propagation"`
	Root        nativeSpanInfo    `json:"root"`
	Current     nativeSpanInfo    `json:"current"`
}
type nativeTraceOp struct {
	ID         string         `json:"id"`
	Method     string         `json:"method"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Name       string         `json:"name,omitempty"`
	Status     map[string]any `json:"status,omitempty"`
	Link       map[string]any `json:"link,omitempty"`
}

// Keep annotations below the callback client's header budget. Telemetry is
// best effort: oversized operations and a full queue are dropped, never allowed
// to prevent application response delivery. Both retained memory and wire bytes
// are bounded, including callbacks which never flush a response.
const nativeTraceBudget = 3 * 1024

type nativeTraceSink struct {
	mu    sync.Mutex
	ops   []json.RawMessage
	bytes int
}
type nativeTraceSinkKey struct{}

func (s *nativeTraceSink) append(op nativeTraceOp) {
	raw, err := json.Marshal(op)
	if err != nil || len(raw)+2 > nativeTraceBudget {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bytes+len(raw)+1 > nativeTraceBudget-2 {
		return
	}
	s.ops = append(s.ops, raw)
	s.bytes += len(raw) + 1
}
func (s *nativeTraceSink) drain() []json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	ops := s.ops
	s.ops = nil
	s.bytes = 0
	return ops
}

type nativeSpan struct {
	trace.Span
	info nativeSpanInfo
	sink *nativeTraceSink
}

func (s *nativeSpan) SpanContext() trace.SpanContext       { return nativeSpanContext(s.info) }
func (s *nativeSpan) IsRecording() bool                    { return s.info.Recording }
func (s *nativeSpan) TracerProvider() trace.TracerProvider { return otel.GetTracerProvider() }
func (s *nativeSpan) send(op nativeTraceOp) {
	if s.info.Recording {
		op.ID = s.info.SpanID
		s.sink.append(op)
	}
}
func (s *nativeSpan) SetAttributes(attrs ...attribute.KeyValue) {
	values := map[string]any{}
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.AsInterface()
	}
	s.send(nativeTraceOp{Method: "attributes", Attributes: values})
}
func (s *nativeSpan) SetName(name string) { s.send(nativeTraceOp{Method: "name", Name: name}) }
func (s *nativeSpan) SetStatus(code codes.Code, message string) {
	n := 0
	if code == codes.Ok {
		n = 1
	}
	if code == codes.Error {
		n = 2
	}
	s.send(nativeTraceOp{Method: "status", Status: map[string]any{"code": n, "message": message}})
}
func (s *nativeSpan) AddEvent(name string, options ...trace.EventOption) {
	cfg := trace.NewEventConfig(options...)
	attrs := map[string]any{}
	for _, a := range cfg.Attributes() {
		attrs[string(a.Key)] = a.Value.AsInterface()
	}
	s.send(nativeTraceOp{Method: "event", Name: name, Attributes: attrs})
}
func (s *nativeSpan) RecordError(err error, options ...trace.EventOption) {
	if err == nil {
		return
	}
	options = append(options, trace.WithAttributes(attribute.String("exception.type", "Error"), attribute.String("exception.message", err.Error())))
	s.AddEvent("exception", options...)
}
func (s *nativeSpan) AddLink(link trace.Link) {
	attrs := map[string]any{}
	for _, a := range link.Attributes {
		attrs[string(a.Key)] = a.Value.AsInterface()
	}
	context := map[string]any{"traceId": link.SpanContext.TraceID().String(), "spanId": link.SpanContext.SpanID().String(), "traceFlags": int(link.SpanContext.TraceFlags())}
	s.send(nativeTraceOp{Method: "link", Link: map[string]any{"context": context, "attributes": attrs}})
}
func (s *nativeSpan) End(...trace.SpanEndOption) { s.send(nativeTraceOp{Method: "end"}) }

func nativeTracingContext(ctx context.Context, header string) context.Context {
	if header == "" {
		return ctx
	}
	raw, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return ctx
	}
	var input nativeTraceInput
	if json.Unmarshal(raw, &input) != nil {
		return ctx
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(input.Propagation))
	sink := &nativeTraceSink{}
	root, current := trace.Span(noSpan), trace.Span(noSpan)
	if input.Enabled {
		root = &nativeSpan{Span: noSpan, info: input.Root, sink: sink}
		current = &nativeSpan{Span: noSpan, info: input.Current, sink: sink}
	}
	ctx = context.WithValue(ctx, tracingKey{}, tracingState{enabled: input.Enabled, root: root, native: true})
	if input.Enabled {
		ctx = context.WithValue(ctx, nativeTraceSinkKey{}, sink)
	}
	return trace.ContextWithSpan(ctx, current)
}
func inheritNativeTracing(ctx, from context.Context) context.Context {
	state, ok := from.Value(tracingKey{}).(tracingState)
	if !ok || !state.native {
		return ctx
	}
	// The continuation has its own cancellation/values, but retains standard
	// propagated context (for example application-configured baggage).
	carrier := propagation.MapCarrier{}
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(from, carrier)
	ctx = propagator.Extract(ctx, carrier)
	ctx = context.WithValue(ctx, tracingKey{}, state)
	if sink, ok := from.Value(nativeTraceSinkKey{}).(*nativeTraceSink); ok {
		ctx = context.WithValue(ctx, nativeTraceSinkKey{}, sink)
	}
	return trace.ContextWithSpan(ctx, trace.SpanFromContext(from))
}

// Headers carry annotations before the first bytes; trailers carry mutations
// made after a streaming callback starts. Neither header is forwarded publicly.
type nativeTraceWriter struct {
	http.ResponseWriter
	sinks []*nativeTraceSink
	sent  bool
}

func (w *nativeTraceWriter) add(ctx context.Context) {
	sink, _ := ctx.Value(nativeTraceSinkKey{}).(*nativeTraceSink)
	if sink == nil {
		return
	}
	for _, existing := range w.sinks {
		if existing == sink {
			return
		}
	}
	w.sinks = append(w.sinks, sink)
}
func (w *nativeTraceWriter) annotations() string {
	var ops []json.RawMessage
	bytes := 2 // enclosing array
	for _, sink := range w.sinks {
		for _, op := range sink.drain() {
			if bytes+len(op)+1 > nativeTraceBudget {
				continue
			}
			ops = append(ops, op)
			bytes += len(op) + 1
		}
	}
	if len(ops) == 0 {
		return ""
	}
	raw, _ := json.Marshal(ops)
	return base64.StdEncoding.EncodeToString(raw)
}

func (w *nativeTraceWriter) WriteHeader(status int) {
	if !w.sent {
		w.sent = true
		if len(w.sinks) > 0 {
			w.Header().Add("Trailer", "X-Skgo-Tracing-After")
			w.Header().Del("Content-Length")
		}
		if value := w.annotations(); value != "" {
			w.Header().Set("X-Skgo-Tracing", value)
		}
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *nativeTraceWriter) Write(raw []byte) (int, error) {
	if !w.sent {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(raw)
}
func (w *nativeTraceWriter) Flush() {
	if !w.sent {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w *nativeTraceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *nativeTraceWriter) finish() {
	if value := w.annotations(); value != "" {
		if !w.sent {
			w.Header().Set("X-Skgo-Tracing", value)
		} else {
			w.Header().Set("X-Skgo-Tracing-After", value)
		}
	}
}

// The private development callback transports diagnostics separately from the Go
// handleError result. Kit sees the original error class; public responses keep
// the already-sanitized body and never rerun the Go hook.
type nativeErrorDiagnostic struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Status  int    `json:"status,omitempty"`
	Stack   string `json:"stack,omitempty"`
	Source  string `json:"source,omitempty"`
}

func nativeErrorFor(ctx context.Context, err error) *nativeErrorDiagnostic {
	if err == nil {
		return nil
	}
	var panicError *tracingPanicError
	if errors.As(err, &panicError) {
		return &nativeErrorDiagnostic{Kind: "unknown", Message: fmt.Sprint(panicError.value)}
	}
	var known *HTTPError
	if errors.As(err, &known) {
		return &nativeErrorDiagnostic{Kind: "known", Message: known.Message, Status: known.Status}
	}
	diagnostic := &nativeErrorDiagnostic{Kind: "unknown", Message: err.Error()}
	var loadError *developmentLoadError
	if errors.As(err, &loadError) {
		diagnostic.Source = loadError.source
		diagnostic.Stack = authoredDevelopmentStack(loadError.stack)
	}
	return diagnostic
}
func developmentAction(ctx context.Context, action *PageAction, name, route string) (value any, err error) {
	defer func() {
		if panicValue := recover(); panicValue != nil {
			err = fmt.Errorf("%v", panicValue)
		}
	}()
	return runTracedAction(ctx, action, name, route)
}

// Ordinary Go errors have no return-site stack. Keep the authored declaration
// separately, and retain actual panic frames rather than substituting that site.
type developmentLoadError struct {
	err    error
	source string
	stack  string
}

func (e *developmentLoadError) Error() string { return e.err.Error() }
func (e *developmentLoadError) Unwrap() error { return e.err }

var generatedDevelopmentFrame = regexp.MustCompile(`(?m)^\s*[^\n]*?/_links/([a-z2-7]+)/([^/\n]+\.go):([0-9]+)`)

func authoredDevelopmentStack(stack string) string {
	return generatedDevelopmentFrame.ReplaceAllStringFunc(stack, func(frame string) string {
		match := generatedDevelopmentFrame.FindStringSubmatch(frame)
		route, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(match[1]))
		if err != nil || !strings.HasPrefix(string(route), "src/routes") {
			return frame
		}
		return "\t" + string(route) + "/" + match[2] + ":" + match[3]
	})
}
