---
title: "skgo: actions"
summary: "Form actions in Go: Action, DefaultAction, Fail and the generated submit path."
---

The Go side of a `+page.server.ts` `actions` export. Each action is a Go
function marked with `Action` or one of its variants; a browser with
JavaScript off posts the form and gets a full page back, a browser with
JavaScript on gets the result without a reload, exactly as in Kit. Kit's guide
is [Form actions](form-actions).

`Fail` returns validation data with a 4xx status and keeps the page rendered;
a `*Redirect` from the [errors and redirects](skgo-errors) page sends the
browser elsewhere after a successful submit. `SubmitForm` and `FormClient` are
the server-side client the generated Swift and Go callers use to post an
action the way a browser would.

## Action

```go
func Action[Out any](fn func(context.Context) (Out, error)) Marker
```

Action publishes a named page form action beside a page load in page.server.go. The action reads its submission from EventFrom(ctx).Request(). Its result is a plain object, carried through the application's transport hook.

## DefaultAction

```go
func DefaultAction[Out any](fn func(context.Context) (Out, error)) Marker
```

DefaultAction publishes the unnamed action on a page with no named actions.

## ActionWithFailure

```go
func ActionWithFailure[Out, Failure any](fn func(context.Context) (Out, error), failure Failure) Marker
```

ActionWithFailure declares the typed data returned by Fail. The handler is still implemented only in Go; the second argument supplies its failure type to the generated Kit action export.

## ActionNoData

```go
func ActionNoData(fn func(context.Context) error) Marker
```

ActionNoData declares an action whose successful result has no form data.

## Fail

```go
func Fail[Data any](status int, data Data) error
```

Fail returns a Kit validation failure rather than an error page. Its data is retained in the form channel for native and enhanced submissions.

## SubmitForm

```go
func SubmitForm[In, Out any](ctx context.Context, client FormClient, id string, input In, decode func(any) (Out, error)) (Out, error)
```

SubmitForm performs one typed Form submission. The generated client supplies the endpoint identity and its result decoder; callers never handle the wire.

## FormClient

```go
type FormClient struct {
	BaseURL    string
	HTTPClient *http.Client
}
```

FormClient sends enhanced Form submissions to the same endpoint used by Kit. HTTPClient may use a custom Transport (including a Unix socket dialer). Each call makes exactly one HTTP request; a lost reply is never replayed.

## DecodeForm

```go
func DecodeForm(arg any, into any) error
```

DecodeForm assigns a form submission onto the handler's argument type.

A form is the one remote kind whose argument is not a devalue payload: a hydrated kit page posts `application/x-sveltekit-formdata`, whose POJO can carry an uploaded File. A File is not a JSON value and polytype does not describe one, so the generated closure calls this instead of a generated decoder; into is a pointer to the handler's own argument type, so the shape is still fixed by the Go signature and never by a type parameter here.

## Actions

```go
type Actions struct {
	// contains filtered or unexported fields
}
```

## ActionSpec

```go
type ActionSpec struct {
	Module string
	Name   string
	Run    func(context.Context) (any, error)
}
```

## PageAction

```go
type PageAction struct {
	// contains filtered or unexported fields
}
```

PageAction is a generated Go action registration.

## NewActions

```go
func NewActions(actions ...*PageAction) (*Actions, error)
```

## NewPageAction

```go
func NewPageAction(spec ActionSpec) *PageAction
```
