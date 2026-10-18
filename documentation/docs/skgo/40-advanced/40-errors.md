---
title: Errors
summary: Expected and unexpected errors from Go code, error pages, and where handleError runs.
proof:
  - TestRemoteValidationHookAtHandler
  - TestNativeValidationHookAtCallbackAndFrontDoor
  - TestDevelopmentSnapshotUsesLiveKitGraphAtGoHandler
  - TestDocumentErrorWithNoHookKeepsTheFallbackExactly
  - TestDocumentErrorClassifiesAnOrdinaryGoErrorAsUnknown
  - TestDocumentErrorRecoversAPanickingHook
  - TestAnUnknownPathIsAnsweredWithTheRenderedErrorPage
  - TestADataErrorCarriesWhatTheAppsHandleErrorHookChose
  - TestAFatalServerRouteUsesTheAppsErrorTemplate
  - TestAPanickingEndpointNegotiatesJSONAndErrorTemplate
  - TestAPanickingLoadAnswersInsteadOfResettingTheConnection
  - TestALoadThatFailsInTheRootLayoutIsKitsStaticErrorPage
---

Errors in SKGo follow [Kit's model](https://svelte.dev/docs/kit/errors). An error you meant to send has a status and a message the visitor may see. Any other error is reported as a 500 with the message `Internal Error`, and its text never reaches the browser. What differs is how Go code produces each kind, and which Go hook decides what the visitor sees.

## Error objects

The error a page receives is an `App.Error`: `{ status, message }` plus any fields your [`handleError`](hooks#handleError) hook adds. SKGo builds it in Go and hands it to Kit, so `page.error` in `+error.svelte` and the `error` of a `failed` snippet have the same shape as in any Kit app.

Kit's hook sees one of four kinds: `'app'`, `'framework'`, `'validation'` or `'unknown'`. The Go hook sees `"app"`, `"validation"` and `"unknown"`. `"app"` is an error with a status: your own `*skgo.HTTPError`, or one SKGo raised itself. `"validation"` identifies rejected remote arguments. `"unknown"` covers everything else.

## App errors

Return an `*skgo.HTTPError` to send an error on purpose. `skgo.Errorf` builds one, as Kit's [`error`](https://svelte.dev/docs/kit/@sveltejs-kit#error) does:

```go
/// file: src/routes/account/statement/page.server.go
/// include: example/web/src/routes/account/statement/page.server.go
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

A remote argument that does not decode into its Go parameter type is rejected before the application function runs. Set `RemoteConfig.HandleError` to customize its public result in production and native development. Kit 3 calls this hook with `kind: 'validation'`; the old `handleValidationError` name was removed.

The Go hook receives `caught.Kind == "validation"`, status `400`, message `"Bad Request"`, and private `caught.Issues`. Returning `nil` keeps those safe defaults. A partial map overrides only the fields you name and may add application fields such as a support id. Decoder diagnostics remain private unless you explicitly publish them. The hook's request event is read-only.

Generated decoders use `skgo.BadRequest(err)` for this refusal. A missing required argument follows the same path. Supplying an argument to a function declared without one remains an ordinary app error, as in Kit; malformed wire payloads are also refused before schema validation.

## Unknown errors

Any other non-nil `error` a load, action or render returns is unknown. The visitor gets `{ "status": 500, "message": "Internal Error" }`, and the error's own text stays on the server:

```go
/// file: src/routes/error/unexpected/page.server.go
/// include: example/web/src/routes/error/unexpected/page.server.go#unexpected
```

`SSROptions.OnError` is told about failures the visitor does not see in full, such as a render that threw or a load value that could not be encoded. When it is nil, SKGo logs them.

### Panics

A panic in a load, action, endpoint, remote function or `handle` is recovered. The request gets a 500 instead of a reset connection, other requests carry on, and the panic goes to that configuration's `OnPanic` callback, or to the log when `OnPanic` is nil. `LoadConfig`, `RemoteConfig` and `HandleConfig` take `func(id string, value any, stack []byte)`; `EndpointConfig` takes `func(routeID, method string, value any, stack []byte)`.

## Where handleError runs

The Go `handleError` hook runs, for any error but a redirect, on:

- a load or action that fails while Go renders a document;
- a load that fails while answering a client navigation's `__data.json`;
- a path that matches no route;
- an error `handle` returns for a document request; data and remote requests get JSON without the hook;
- a panic in a `+server` route, answered as JSON or as the fallback error page depending on `Accept`;
- rejected remote schema arguments, through `RemoteConfig.HandleError`.

It does not run:

- for other remote-function failures in production. A remote function's `*HTTPError` reaches the caller as it is, and any other error becomes `{ "status": 500, "message": "Internal Error" }`. Under [`skgo dev`](development-server) these other failures go through the load configuration's hook.
- while [prerendering](prerendering) ([#278](https://github.com/tylergannon/skgo/issues/278)).

Set it as `HandleError` on the remote, load, endpoint and handle configurations in `server.go`; [Server composition](server-composition) shows where. The example application uses it to attach a support id to every failure and to replace the message of an unknown error:

```go
/// file: server.go
/// include: example/server.go#handle-error
```

`caught.Status` and `caught.Message` are what the visitor gets if the hook returns `nil`. `caught.Kind` is `"app"`, `"validation"` or `"unknown"`. `caught.Err` is the original error, set only for `"unknown"`; classify it with `errors.As`. The map you return overrides fields: `"status"` must be an `int` and `"message"` a `string`, and other keys become extra `App.Error` fields. The hook's event is read-only, so it cannot set cookies. A hook that panics is logged, and the message becomes `"Internal Error"`, even for an `"app"` error.

## Error boundaries

Errors in loads and rendering bubble up to the nearest `+error.svelte`, and a [`<svelte:boundary>`](https://svelte.dev/docs/svelte/svelte-boundary) catches errors at a finer grain, as in [Kit](https://svelte.dev/docs/kit/errors#Error-boundaries). Kit's rule that a caught error still sets the status of the whole document applies during server rendering. A page whose boundary catches a query's `skgo.Errorf(409, "The sensor is being calibrated")` renders its `failed` snippet, and the document is answered with status 409.

A redirect from a remote query that a component calls during server rendering is not an error. It redirects the whole document:

```go
/// file: src/routes/error/redirect/redirect.remote.go
/// include: example/web/src/routes/error/redirect/redirect.remote.go#redirect
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
