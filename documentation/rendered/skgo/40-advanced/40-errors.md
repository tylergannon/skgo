---
title: "Errors"
summary: "Expected and unexpected errors from Go code, error pages, and where handleError runs."
---

Errors in SKGo follow [Kit's model](https://svelte.dev/docs/kit/errors). An error you meant to send has a status and a message the visitor may see. Any other error is reported as a 500 with the message `Internal Error`, and its text never reaches the browser. What differs is how Go code produces each kind, and which Go hook decides what the visitor sees.

## Error objects

The error a page receives is an `App.Error`: `{ status, message }` plus any fields your [`handleError`](hooks#handleError) hook adds. SKGo builds it in Go and hands it to Kit, so `page.error` in `+error.svelte` and the `error` of a `failed` snippet have the same shape as in any Kit app.

Kit's hook sees one of four kinds: `'app'`, `'framework'`, `'validation'` or `'unknown'`. The Go hook sees two. `"app"` is an error with a status: your own `*skgo.HTTPError`, or one SKGo raised itself. `"unknown"` covers everything else.

## App errors

Return an `*skgo.HTTPError` to send an error on purpose. `skgo.Errorf` builds one, as Kit's [`error`](https://svelte.dev/docs/kit/@sveltejs-kit#error) does:

```go
/// file: src/routes/account/statement/page.server.go
// Package statement is the page whose load fails, so that a visitor can see
// what a failed load does: an error page with the status and message the Go
// function chose, inside the layout that declared the error page.
package statement

import (
	"github.com/tylergannon/skgo"
)

// PageData is never produced.
type PageData struct {
	// Balance would be the amount owed.
	Balance int `json:"balance"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{}, skgo.Errorf(402, "Your account is in arrears")
}

var _ = skgo.Load(pageLoad)
```

The response status becomes 402, and the nearest [`+error.svelte`](routing#error) renders with that status and message:

```svelte
<!--- file: src/routes/+error.svelte --->
<script lang="ts">
	import { page } from '$app/state';
</script>

<h1>{page.status}: {page.error?.message}</h1>
```

The same error works from form actions, `handle` and remote functions. A `+server` route writes its own response with `net/http`, so it sets the status itself. `HTTPError` has only `Status` and `Message`. Kit's third argument to `error()` has no Go counterpart; extra fields come from `handleError`.

## Framework errors

SKGo raises Kit's framework errors itself, with Kit's messages. A path that matches no route is a 404 `Not Found`, rendered with the root error page inside the root layout. A POST to a remote form id that does not exist is a 405.

## Validation errors

Set `RemoteConfig.HandleError` to customize a rejected generated remote argument before the Go body runs. Kit 3.1.0 uses the ordinary `handleError` hook with `caught.Kind == "validation"`; the historical separate `handleValidationError` hook is gone.

`caught.Status` and `caught.Message` are the safe defaults `400` and `Bad Request`. `caught.Issues` holds private decoder diagnostics as messages. Return a map to override the public message or add application fields; omitted fields retain their defaults. Return `nil` to keep the safe result. A panicking hook keeps status 400 and replaces the message with `Internal Error`.

The example chooses `Choose a catalog name.` and a `reference` for its catalog remote:

```go
/// file: src/routes/validation/catalog.remote.go
package validation

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tylergannon/skgo"
)

var bodyCalls atomic.Int64

type CatalogInput struct {
	Name string `json:"name"`
}

func validateCatalog(ctx context.Context, input CatalogInput) (string, error) {
	calls := bodyCalls.Add(1)
	return fmt.Sprintf("Go accepted %s|body-calls=%d", input.Name, calls), nil
}

var _ = skgo.Query(validateCatalog)

func validationBodyCalls(ctx context.Context) (int64, error) { return bodyCalls.Load(), nil }

var _ = skgo.Query(validationBodyCalls)
```

The same hook shapes query, command, batch and live schema failures in development and production. A batch argument refusal rejects the batch before its body runs; a live failure arrives in its error frame. Production prerender remotes serve their built files, while development validates calls normally.

An ordinary application `skgo.BadRequest(err)`, a malformed transport payload, an argument sent to a no-argument function and a form's business `Invalid` issues do not become argument-schema failures. Form submissions keep their normal issue result. Decoder details are never copied to the public result automatically.

## Unknown errors

Any other non-nil `error` a load, action or render returns is unknown. The visitor gets `{ "status": 500, "message": "Internal Error" }`, and the error's own text stays on the server:

```go
/// file: src/routes/error/unexpected/page.server.go
func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{}, errors.New("the connection string is postgres://ada:hunter2@db")
}
```

`SSROptions.OnError` is told about failures the visitor does not see in full, such as a render that threw or a load value that could not be encoded. When it is nil, SKGo logs them.

### Panics

A panic in a load, action, endpoint, remote function or `handle` is recovered. The request gets a 500 instead of a reset connection, other requests carry on, and the panic goes to that configuration's `OnPanic` callback, or to the log when `OnPanic` is nil. `LoadConfig`, `RemoteConfig` and `HandleConfig` take `func(id string, value any, stack []byte)`; `EndpointConfig` takes `func(routeID, method string, value any, stack []byte)`.

## Where handleError runs

The Go `handleError` hook runs, for any error but a redirect, on:

- a load or action that fails while Go renders a document;
- a load that fails while answering a client navigation's `__data.json`;

- a remote function that returns an error or panics, including streamed query errors and query refreshes;
- an error a Svelte boundary catches during server rendering, in production and development;

- a path that matches no route;
- an error `handle` returns for a document request; data and remote requests get JSON without the hook;
- a panic in a `+server` route, answered as JSON or as the fallback error page depending on `Accept`.

It does not run:



- while [prerendering](prerendering) ([#278](https://github.com/tylergannon/skgo/issues/278)).


Set the same `HandleError` on the load, remote, endpoint and handle configurations in `server.go`; [Server composition](server-composition) shows where. The load configuration supplies the hook for Svelte component boundaries. Already-handled errors reuse their public body without running the hook again. The example application uses it to attach a support id to every failure and to replace the message of an unknown error:


```go
/// file: server.go
// HandleError is the app's `handleError` hook — kit's own contract, mirrored:
// it runs for page and remote-function failures, expected or not
// (`exports/hooks/public.d.ts`: "runs for every error thrown ... except
// redirects"), and whatever it returns is merged over what the error already
// carries.
//
// Every failure gets a support id, which is the one thing a real error page
// almost always adds and kit's own default has no room for. An error nobody
// meant to happen also loses its own message here: this app makes no promise
// about what an arbitrary Go error's text might contain, so the visitor is
// told something a person wrote instead of it. caught.Err still carries the
// real one, for a hook that wants to log it before replacing it.
func HandleError(ctx context.Context, caught skgo.CaughtError) map[string]any {
	extra := map[string]any{"supportId": supportID}
	if caught.Kind == "validation" && strings.HasSuffix(skgo.EventFrom(ctx).Request().URL.Path, "/validateCatalog") {
		return map[string]any{"message": "Choose a catalog name.", "reference": "catalog-validation-166"}
	}
	if caught.Kind == "unknown" {
		extra["message"] = "Something went wrong on our end."
	}
	return extra
}
```

`caught.Status` and `caught.Message` are what the visitor gets if the hook returns `nil`. `caught.Kind` is `"app"`, `"unknown"` or `"validation"`. Validation carries separate private `caught.Issues`. `caught.Err` is the original error, set only for `"unknown"`; classify it with `errors.As`. The map you return overrides fields: `"status"` must be an `int` and `"message"` a `string`, and other keys become extra `App.Error` fields. The hook's event is read-only, so it cannot set cookies. A hook that panics is logged, and the message becomes `"Internal Error"`, even for an `"app"` error.

## Error boundaries

Errors in loads and rendering bubble up to the nearest `+error.svelte`, and a [`<svelte:boundary>`](https://svelte.dev/docs/svelte/svelte-boundary) catches errors at a finer grain, as in [Kit](https://svelte.dev/docs/kit/errors#Error-boundaries). Kit's rule that a caught error still sets the status of the whole document applies during server rendering. A page whose boundary catches a query's `skgo.Errorf(409, "The sensor is being calibrated")` renders its `failed` snippet, and the document is answered with status 409.

A redirect from a remote query that a component calls during server rendering is not an error. It redirects the whole document:

```go
/// file: src/routes/error/redirect/redirect.remote.go
func whereTo(_ context.Context) (Destination, error) {
	return Destination{}, &skgo.Redirect{Status: 307, Location: "/about"}
}
```

## Responses

An error returned from `handle`, and a panic in a `+server` route, get JSON or the fallback error page, depending on the request's `Accept` header, as in [Kit](https://svelte.dev/docs/kit/errors#Responses). The fallback page is your `src/error.html`, or Kit's default, with `%sveltekit.status%` and `%sveltekit.error.message%` replaced. `server.go` passes the renderer's template to the handle configuration as `ErrorTemplate` and to the endpoints with `SetErrorTemplate`.

When the root layout itself fails, no error page can render, so the fallback page answers.

## Type safety

Declare extra fields on `App.Error` in `src/app.d.ts`, as in [Kit](https://svelte.dev/docs/kit/errors#Type-safety):

```ts
/// file: src/app.d.ts
declare global {
	namespace App {
		interface Error {
			supportId?: string;
		}
	}
}

export {};
```

Nothing connects the declaration to your Go hook. Keep the keys the hook returns and the fields declared here the same, and mark them optional unless the hook sets them for every error.

## Further reading

- [skgo errors](skgo-errors), the reference for `HTTPError`, `BadRequest` and `Redirect`
- [Loading data](load#Errors), for errors from loads
