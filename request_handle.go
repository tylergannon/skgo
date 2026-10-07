package skgo

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"
	"sync/atomic"
)

// A sequence's guarded HTTP boundary must report a panic with the failing
// hook's incoming binding, just as it reports an ordinary refusal.
type requestHookPanic struct {
	ctx   context.Context
	value any
	stack []byte
}

func callRequestHook[P, L any](h RequestMiddleware[P, L], ctx context.Context, event RequestEvent[P, L], resolve RequestResolve[P, L]) (response *http.Response, err error) {
	defer func() {
		if value := recover(); value != nil {
			if _, ok := value.(*requestHookPanic); ok {
				panic(value)
			}
			panic(&requestHookPanic{ctx: ctx, value: value, stack: debug.Stack()})
		}
	}()
	return h(ctx, event, resolve)
}

type requestRefusal struct {
	ctx context.Context
	err error
}

func (e *requestRefusal) Error() string { return e.err.Error() }
func (e *requestRefusal) Unwrap() error { return e.err }
func requestHookError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*requestRefusal); ok {
		return err
	}
	return &requestRefusal{ctx: ctx, err: err}
}

type localsKey struct{}

// RequestLocals returns the application's selected object, or nil outside a
// bound request. A mismatched type is a programming error, never a new store.
func RequestLocals[L any](ctx context.Context) *L {
	value := ctx.Value(localsKey{})
	if value == nil {
		return nil
	}
	locals, ok := value.(*L)
	if !ok {
		panic(fmt.Sprintf("skgo: requested locals %v, configured locals %T", reflect.TypeFor[*L](), value))
	}
	return locals
}

// RequestResolve forwards a value event to the next hook or application.
type RequestResolve[P, L any] func(context.Context, RequestEvent[P, L], ...ResolveOptions) (*http.Response, error)

// RequestMiddleware is a value-event hook with explicit downstream forwarding.
type RequestMiddleware[P, L any] func(context.Context, RequestEvent[P, L], RequestResolve[P, L]) (*http.Response, error)

// RequestHandle returns the event a before-only hook wants to forward.
type RequestHandle[P, L any] func(context.Context, RequestEvent[P, L]) (RequestEvent[P, L], error)

func (h RequestHandle[P, L]) Middleware() RequestMiddleware[P, L] {
	if h == nil {
		return nil
	}
	return func(ctx context.Context, event RequestEvent[P, L], resolve RequestResolve[P, L]) (*http.Response, error) {
		selected, err := h(ctx, event)
		if err != nil {
			return nil, err
		}
		return resolve(ctx, selected)
	}
}

func bindRequestEvent[P, L any](ctx context.Context, event RequestEvent[P, L]) (context.Context, RequestEvent[P, L]) {
	ctx = context.WithValue(ctx, localsKey{}, event.Locals)
	if event.Event != nil {
		core := *event.Event
		core.req = core.req.WithContext(ctx)
		event.Event = &core
		ctx = withEvent(ctx, event.Event)
	}
	return ctx, event
}

func guardedRequestResolve[P, L any](next RequestResolve[P, L]) RequestResolve[P, L] {
	var called atomic.Bool
	return func(ctx context.Context, event RequestEvent[P, L], options ...ResolveOptions) (*http.Response, error) {
		if !called.CompareAndSwap(false, true) {
			return nil, fmt.Errorf("skgo: resolve was already called for this request")
		}
		if event.Locals == nil {
			return nil, Errorf(500, "skgo: Locals must not be nil when resolving a request")
		}
		if _, err := singleResolveOptions(options); err != nil {
			return nil, err
		}
		ctx, event = bindRequestEvent(ctx, event)
		return next(ctx, event, options...)
	}
}

// RequestSequence retains each hook's incoming context binding. Only resolve
// forwards replacements; mutations of a shared object follow Go pointer rules.
func RequestSequence[P, L any](hooks ...RequestMiddleware[P, L]) RequestMiddleware[P, L] {
	var chain []RequestMiddleware[P, L]
	for _, h := range hooks {
		if h != nil {
			chain = append(chain, h)
		}
	}
	return func(ctx context.Context, event RequestEvent[P, L], resolve RequestResolve[P, L]) (*http.Response, error) {
		var apply func(int, context.Context, RequestEvent[P, L], *ResolveOptions) (*http.Response, error)
		apply = func(i int, ctx context.Context, event RequestEvent[P, L], parent *ResolveOptions) (*http.Response, error) {
			if i == len(chain) {
				if parent == nil {
					return resolve(ctx, event)
				}
				return resolve(ctx, event, *parent)
			}
			response, err := callRequestHook(chain[i], ctx, event, guardedRequestResolve(func(ctx context.Context, selected RequestEvent[P, L], options ...ResolveOptions) (*http.Response, error) {
				own, err := singleResolveOptions(options)
				if err != nil {
					return nil, err
				}
				return apply(i+1, ctx, selected, composeResolveOptions(own, parent))
			}))
			return response, requestHookError(ctx, err)
		}
		return apply(0, ctx, event, nil)
	}
}

// HookValues returns the already converted matched route snapshot. It never
// invokes a matcher and is separate from remote caller permissions.
func HookValues(event *Event) (string, map[string]any) {
	if event == nil || event.hook == nil {
		return "", nil
	}
	values := make(map[string]any, len(event.hook.converted))
	for k, v := range event.hook.converted {
		values[k] = v
	}
	return event.hook.routeID, values
}

// Intercept is the application's single locals allocator, including when the
// selected hook is nil. Raw Intercept still owns response and refusal semantics.
func (h RequestMiddleware[P, L]) Intercept(cfg HandleConfig, makeParams func(*Event) (P, error), next http.Handler) http.Handler {
	middleware := Middleware(func(ctx context.Context, core *Event, resolve Resolve) (*http.Response, error) {
		params, err := makeParams(core)
		if err != nil {
			return nil, err
		}
		event := RequestEvent[P, L]{Event: core, Params: params, Locals: RequestLocals[L](ctx)}
		forward := guardedRequestResolve(func(ctx context.Context, selected RequestEvent[P, L], options ...ResolveOptions) (*http.Response, error) {
			return resolve(ctx, options...)
		})
		if h == nil {
			return forward(ctx, event)
		}
		return h(ctx, event, forward)
	})
	return middleware.intercept(cfg, next, func(ctx context.Context) context.Context {
		return context.WithValue(ctx, localsKey{}, new(L))
	})
}
