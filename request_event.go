package skgo

import (
	"context"
	"reflect"
)

// RequestEvent pairs a request with generated typed params and application locals.
// Page and layout loads use their generated RouteParams and LayoutParams domains.
// Commands and forms use the generated application-wide params.Params.
type RequestEvent[Params, Locals any] struct {
	*Event
	Params Params
	Locals *Locals
}

// Context carries cancellation and the existing request-scoped helpers.
func (e RequestEvent[P, L]) Context() context.Context {
	event := e.Event
	if event.remote {
		// Context helpers and direct nested queries cannot recover the explicit
		// command/form caller. Cookies, cancellation and refresh state stay shared.
		derived := *event
		derived.caller, derived.hook, derived.load, derived.params = nil, nil, nil, nil
		derived.query, derived.mutable = true, false
		derived.req = event.req.WithContext(withEvent(event.req.Context(), &derived))
		event = &derived
	}
	return withEvent(e.Request().Context(), event)
}

// ParamMatcher is a generated adapter for a plain Go func(string) (T, bool).
// Only the bool decides whether a route candidate is accepted.
type ParamMatcher func(string) (any, bool)

// TrackLoadParam records a generated accessor's read on this load invocation.
func TrackLoadParam(e *Event, name string) {
	if e != nil && e.load != nil {
		e.load.uses.add(&e.load.uses.params, name)
	}
}

// LoadParamValue populates a generated RouteParams field without recording a
// dependency. Matching has already converted the value to the matcher's type.
func LoadParamValue[T any](e *Event, name string) T {
	var zero T
	if e == nil || e.load == nil {
		return zero
	}
	value, exists := e.load.shared.converted[name]
	if !exists {
		// Prerender bridges and legacy registrations can supply string params.
		value, exists = e.load.shared.params[name]
	}
	if !exists {
		return zero
	}
	// An accepted nil interface has no dynamic type to assert. Only the
	// declared interface domain permits this case; a nil conversion for any
	// other T remains a mismatch and must fail instead of becoming zero.
	if value == nil && reflect.TypeFor[T]().Kind() == reflect.Interface {
		return zero
	}
	return value.(T)
}

// OptionalLoadParamValue populates an optional field without tracking, preserving
// the distinction between absence and an accepted zero, false or empty value.
func OptionalLoadParamValue[T any](e *Event, name string) *T {
	if e == nil || e.load == nil {
		return nil
	}
	if _, ok := e.load.shared.params[name]; !ok {
		return nil
	}
	value := LoadParamValue[T](e, name)
	return &value
}

// LoadRouteIDValue selects a generated layout alternative without recording a
// route dependency. A layout's public RouteID still tracks the matched page.
func LoadRouteIDValue(e *Event) string {
	if e == nil || e.load == nil {
		return ""
	}
	return e.load.shared.routeID
}
