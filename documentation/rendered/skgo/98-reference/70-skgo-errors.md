---
title: "skgo: errors and redirects"
summary: "Errorf, HTTPError, Invalid, Issue, BadRequest and Redirect."
---

How a Go handler says no. `Errorf` returns an error with an HTTP status that
Kit renders through the nearest `+error.svelte`, the way `error()` does.
`Invalid` carries field-level validation messages back to a form or a remote
function's caller, with each message on the field it names, and `BadRequest`
is the 400 for input the handler could not decode. `NewRedirect` sends the
browser elsewhere, from a load, an action or a command. `CaughtError` is what
`HandleError` receives. Kit's guide is [Errors](https://svelte.dev/docs/kit/errors).

## HTTPError

```go
type HTTPError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}
```

HTTPError is an error with a status kit's client understands. Returning one from a remote function produces `{"type":"error","error":{...}}`; any other error defaults to an opaque 500. RemoteConfig.HandleError may customize the public body, as Kit's handleError does.

### Errorf

```go
func Errorf(status int, format string, args ...any) *HTTPError
```

Errorf builds an HTTPError. It is the analogue of kit's `error(status, message)`.

### Error

```go
func (e *HTTPError) Error() string
```

## BadRequest

```go
func BadRequest(detail error) error
```

BadRequest returns an opaque 400 while retaining detail on the server. It is an application refusal, not a generated argument-schema rejection; it does not invoke RemoteConfig.HandleError with kind "validation".

## ValidationIssue

```go
type ValidationIssue struct {
	Message string `json:"message"`
}
```

ValidationIssue is a private argument-decoder diagnostic supplied to HandleError for kind "validation". It reaches the client only if the hook deliberately includes it in its public result.

## Invalid

```go
type Invalid struct {
	Issues []Issue
}
```

Invalid is the error a form handler returns to report validation failures. Kit treats a submission with issues as neither a success nor a server error: the client keeps the page as it is, hangs each message off its field, and — because kit's server returns before it collects them — runs none of the single-flight refreshes the submission asked for.

The visitor's typed input is not sent back and does not need to be. A progressively-enhanced submission never navigates, so the values are still in the inputs; kit's own server skips the input entirely for exactly this case ("we don't need to return the input — it's already there").

### Invalidf

```go
func Invalidf(field, format string, args ...any) *Invalid
```

Invalidf reports one field invalid. It is the common case, and reads at the call site like the check it is:

```go
if !strings.Contains(arg.Email, "@") {
    return Contact{}, skgo.Invalidf("email", "%q is not an email address", arg.Email)
}
```

### Add

```go
func (v *Invalid) Add(field, format string, args ...any) *Invalid
```

Add appends another issue, so a handler can collect every problem with a submission rather than reporting them one round-trip at a time.

### Err

```go
func (v *Invalid) Err() error
```

Err returns v, or nil when nothing was found wrong. It exists so a handler that accumulates issues can end with `return out, issues.Err()` rather than with a length check that is easy to get backwards.

### Error

```go
func (v *Invalid) Error() string
```

## Issue

```go
type Issue struct {
	Field   string
	Message string
}
```

Issue is one validation failure, reported against the field it is about.

Field is the field's path as the form names it: "email" for a top-level field, "author.name" for a nested one, "tags\[0]" for an element of an array field. It must match the path the page used in `form.fields...`, because kit's client matches issues to fields by exactly that string. An empty Field puts the issue on the form as a whole, where `form.fields.allIssues()` finds it.

## Redirect

```go
type Redirect struct {
	// Status is the HTTP status kit's client is told about. Kit requires one
	// of 300..308; zero means 307, kit's own default for a redirect that does
	// not name one.
	Status int
	// Location is where the client is sent.
	Location string
}
```

Redirect is an error that makes the client navigate. It is honoured for queries and live queries; kit's client throws when a \*command\* response carries a redirect, so a Redirect returned from a command is reported as an ordinary error instead.

Status is kit's `redirect(status, location)` status and must be in 300-308, the range kit validates in `exports/index.js`. Build one with NewRedirect, which applies that check and the header-safety check kit's own Redirect constructor applies to the location.

The status does not reach the browser from a remote function, and that is kit's design, not a gap here: kit serialises a remote-function redirect as `{redirect: location}` inside the \*success\* envelope and drops the status (`runtime/server/remote-functions.js`, the `error instanceof Redirect` branch), and its client calls `goto(location)` on it. A live query's frame is `{type:"redirect",location}` and kit's client fabricates a 307 purely so its reconnect loop can recognise it. The status is carried and validated here because it is part of what a caller means, and it becomes observable wherever a redirect is answered as HTTP rather than as an envelope.

### NewRedirect

```go
func NewRedirect(status int, location string) (*Redirect, error)
```

NewRedirect builds a Redirect, applying kit's own two checks: `redirect()` refuses a status outside 300-308 with "Invalid status code", and the Redirect constructor refuses a location that cannot be a header value.

It reports an error rather than panicking, because a redirect target is usually built from a request and a bad one is a request problem.

### Error

```go
func (r *Redirect) Error() string
```

## CaughtError

```go
type CaughtError struct {
	// Kind is "app", "unknown" or "validation".
	Kind string
	// Status and Message are the error's own fallback: what the visitor sees
	// if the hook returns nothing. For "app" this is the app's own status and
	// message; for "unknown" it is always 500 and "Internal Error".
	Status  int
	Message string
	// Err is the original error skgo caught, set only for kind "unknown" — an
	// "app" error's entire body already is Status and Message. A hook that
	// wants to log the real cause of an unknown failure, or classify it
	// further with errors.As, reads it from here; it is never shown to the
	// visitor unless the hook copies something from it into what it returns.
	Err error
	// Issues contains private decoder diagnostics only for kind "validation".
	Issues []ValidationIssue
}
```


CaughtError is the error the application's `handleError` hook is told about — kit's own discriminated `CaughtError` (`exports/hooks/public.d.ts`), narrowed to what a Go port can express without modelling kit's `SvelteKitError` and `ValidationError` classes as distinct Go types the way skgo's `HTTPError` does not yet: "app" for an error already shaped as one — an \*HTTPError, whether the app raised it itself with Errorf or skgo manufactured it for a framework failure such as a 404 — and "unknown" for anything else, which is where kit's real distinction still matters: the visitor is never shown an unknown error's own message, only what the hook explicitly returns.
