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

HTTPError is an error with a status kit's client understands. Returning one from a remote function produces `{"type":"error","error":{...}}`; any other error becomes an opaque 500, exactly as kit does for unexpected throws.

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

BadRequest turns a generated decoder's refusal into the answer kit gives.

Kit's `create_validator` answers 400 to an argument its schema rejects, and to any argument at all when the function was declared without one. skgo's schema is the strict decoder polytype generated from the Go parameter type, so this is the same refusal at the same moment.

What the client is told is kit's opaque "Bad Request". The decoder's own diagnostic — which names a JSON-pointer path into the argument — is kept on the error for the server's logs and never put on the wire, because it describes the shape the server expected and an unauthenticated caller has not earned that. RemoteConfig.HandleError can inspect these private issues and choose a public validation body.

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
	// Kind is "app", "validation" or "unknown".
	Kind string
	// Status and Message are the error's own fallback: what the visitor sees
	// if the hook returns nothing. For "app" this is the app's own status and
	// message; for "validation" it is 400 and "Bad Request"; for "unknown"
	// it is always 500 and "Internal Error".
	Status  int
	Message string
	// Err is the original error skgo caught, set only for kind "unknown" — an
	// "app" error's entire body already is Status and Message. A hook that
	// wants to log the real cause of an unknown failure, or classify it
	// further with errors.As, reads it from here; it is never shown to the
	// visitor unless the hook copies something from it into what it returns.
	Err error
	// Issues contains private decoder diagnostics only for kind "validation".
	// Their messages may describe the expected argument shape. They are never
	// published unless the hook explicitly copies them into its returned body.
	// Field is empty when the decoder supplies no separate form-field path.
	Issues []Issue
}
```

CaughtError is the error passed to the application's HandleError hook. Kind distinguishes an app HTTPError, private argument-validation issues, and an unknown failure whose original error must not reach the visitor.
