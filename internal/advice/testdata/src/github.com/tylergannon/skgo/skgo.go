package skgo

import (
	"context"
	"net/http"
)

type Marker struct{}
type Event struct{}
type CookieOptions struct{}
type Invalid struct{}
type Issue struct {
	Field   string
	Message string
}

func Query(any) Marker                                         { return Marker{} }
func LiveQuery(any) Marker                                     { return Marker{} }
func BatchQuery(any) Marker                                    { return Marker{} }
func Command(any) Marker                                       { return Marker{} }
func Form(any) Marker                                          { return Marker{} }
func Load(any) Marker                                          { return Marker{} }
func EventFrom(context.Context) *Event                         { return nil }
func (e *Event) Request() *http.Request                        { return nil }
func (e *Event) SetCookie(string, string, CookieOptions) error { return nil }
func (e *Event) DeleteCookie(string, CookieOptions) error      { return nil }
func (e *Event) Cookie(string) (string, bool)                  { return "", false }
func (e *Event) Param(string) string                           { return "" }
func (e *Event) URL() string                                   { return "" }
func (e *Event) SearchParam(string) (string, bool)             { return "", false }
func (e *Event) RouteID() string                               { return "" }
func Invalidf(string, string, ...any) *Invalid                 { return nil }
func (i *Invalid) Add(string, string, ...any) *Invalid         { return i }
func RefreshNoArg[Out any](context.Context, func(context.Context) (Out, error)) error {
	return nil
}
func RefreshRequestedNoArg[Out any](context.Context, func(context.Context) (Out, error)) error {
	return nil
}
func ReconnectRequestedNoArg[Out any](context.Context, func(context.Context, func(Out) error) error) error {
	return nil
}
func Refresh[In, Out any](context.Context, func(context.Context, In) (Out, error), In) error {
	return nil
}
func RefreshRequested[In, Out any](context.Context, func(context.Context, In) (Out, error), int) error {
	return nil
}
func ReconnectRequested[In, Out any](context.Context, func(context.Context, In, func(Out) error) error, int) error {
	return nil
}
