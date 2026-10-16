package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Renderer span handles belong to one render. Only Go creates/exports spans;
// the engine's proxy communicates synchronously through this in-process host.
type rendererTracing struct {
	mu       sync.Mutex
	base     context.Context
	next     int
	contexts map[string]context.Context
	spans    map[string]trace.Span
}

func newRendererTracing(ctx context.Context) *rendererTracing {
	t := tracingOf(ctx)
	return &rendererTracing{base: ctx, contexts: map[string]context.Context{"current": ctx, "root": trace.ContextWithSpan(ctx, t.Root)}, spans: map[string]trace.Span{"root": t.Root, "current": t.Current}}
}
func (r *rendererTracing) context(ctx context.Context, id string) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if span := r.spans[id]; span != nil {
		return trace.ContextWithSpan(ctx, span)
	}
	return ctx
}
func (r *rendererTracing) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, span := range r.spans {
		if id != "root" && id != "current" {
			span.End()
		}
	}
}
func traceAttributes(values map[string]any) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(values))
	for key, value := range values {
		switch value := value.(type) {
		case string:
			result = append(result, attribute.String(key, value))
		case bool:
			result = append(result, attribute.Bool(key, value))
		case float64:
			result = append(result, attribute.Float64(key, value))
		case []any:
			if len(value) == 0 {
				result = append(result, attribute.StringSlice(key, nil))
				continue
			}
			switch value[0].(type) {
			case string:
				var list []string
				for _, v := range value {
					if x, ok := v.(string); ok {
						list = append(list, x)
					}
				}
				result = append(result, attribute.StringSlice(key, list))
			case bool:
				var list []bool
				for _, v := range value {
					if x, ok := v.(bool); ok {
						list = append(list, x)
					}
				}
				result = append(result, attribute.BoolSlice(key, list))
			case float64:
				var list []float64
				for _, v := range value {
					if x, ok := v.(float64); ok {
						list = append(list, x)
					}
				}
				result = append(result, attribute.Float64Slice(key, list))
			}
		}
	}
	return result
}
func spanJSON(span trace.Span) map[string]any {
	sc := span.SpanContext()
	return map[string]any{"traceId": sc.TraceID().String(), "spanId": sc.SpanID().String(), "traceFlags": int(sc.TraceFlags()), "traceState": sc.TraceState().String(), "recording": span.IsRecording()}
}
func (r *rendererTracing) call(_ context.Context, raw []byte) ([]byte, error) {
	var input struct {
		Op, ID, Name, Parent string
		Attributes           map[string]any
		Status               struct {
			Code    int
			Message string
		}
		Link struct {
			Context    nativeSpanInfo
			Attributes map[string]any
		}
		Error struct {
			Kind              string
			Status            int
			Message, Location string
		}
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t := tracingOf(r.base)
	if input.Op == "info" {
		return json.Marshal(map[string]any{"enabled": t.Enabled, "root": spanJSON(t.Root), "current": spanJSON(t.Current)})
	}
	if input.Op == "start" {
		ctx := r.contexts[input.Parent]
		if ctx == nil {
			ctx = r.base
		}
		ctx, span := traceOperation(ctx, input.Name, traceAttributes(input.Attributes)...)
		r.next++
		id := fmt.Sprint(r.next)
		r.contexts[id], r.spans[id] = ctx, span
		answer := spanJSON(span)
		answer["id"] = id
		return json.Marshal(answer)
	}
	span := r.spans[input.ID]
	if span == nil {
		span = noSpan
	}
	switch input.Op {
	case "attributes":
		span.SetAttributes(traceAttributes(input.Attributes)...)
	case "event":
		span.AddEvent(input.Name, trace.WithAttributes(traceAttributes(input.Attributes)...))
	case "status":
		code := codes.Unset
		if input.Status.Code == 1 {
			code = codes.Ok
		}
		if input.Status.Code == 2 {
			code = codes.Error
		}
		span.SetStatus(code, input.Status.Message)
	case "name":
		span.SetName(input.Name)
	case "exception":
		span.AddEvent("exception", trace.WithAttributes(traceAttributes(input.Attributes)...))
	case "error":
		var err error
		switch input.Error.Kind {
		case "redirect":
			err = &Redirect{Status: input.Error.Status, Location: input.Error.Location}
		case "known_error":
			err = &HTTPError{Status: input.Error.Status, Message: input.Error.Message}
		default:
			err = fmt.Errorf("%s", input.Error.Message)
		}
		traceError(span, input.Name, err)
	case "link":
		span.AddLink(trace.Link{SpanContext: nativeSpanContext(input.Link.Context), Attributes: traceAttributes(input.Link.Attributes)})
	case "end":
		span.End()
	}
	return []byte("{}"), nil
}
