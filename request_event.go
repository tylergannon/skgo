package skgo

import (
	"context"
	"reflect"
)

// RequestEvent is the explicit event of a route-bound server load. Generated
// route packages alias this implementation with their own RouteParams.
type RequestEvent[Params any] struct {
	*Event
	Params Params
}

// Context carries cancellation and the existing request-scoped helpers.
func (e RequestEvent[P]) Context() context.Context { return withEvent(e.Request().Context(), e.Event) }

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
	// A present nil interface has no dynamic type to assert.
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
