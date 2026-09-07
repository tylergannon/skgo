# handleError hook (#59): the issue's premise was wrong, and two real gaps remain

## Kit does not skip the hook for an expected error

The issue asked for "one scenario for an expected error (kit's `error(404,
...)`) showing the hook is NOT consulted the way kit skips it, if that is what
kit does — map it." It doesn't. `handle_error_and_jsonify`
(`runtime/server/errors.js`) calls `hooks.handleError(input)` for every kind —
app, framework, validation, unknown — and only skips it for `HandledHttpError`,
an error already run through the hook by an earlier layer, which nothing on
skgo's render path produces. Kit's own doc comment on `HandleServerError`
confirms it: "runs for every error thrown while responding to a request,
except redirects." What differs by kind is only the *fallback* the hook's
return is merged over — an app/framework error's own status and message vs.
the fixed `{500, "Internal Error"}` for anything else — never whether the hook
runs. `example.HandleError` (example/server.go) now demonstrates this
directly: it adds a support id to both an `error(418, ...)` and a plain Go
error, and only overrides the message for the latter. The two new ssr.feature
scenarios assert both halves.

## Kind is narrowed to "app"/"unknown", not kit's four

Kit's `CaughtError` discriminates app/framework/validation/unknown. skgo's
`HTTPError` doesn't carry which of those three non-unknown kinds produced it —
a 404 for an unmatched route and an app's own `Errorf(404, ...)` are both just
`*HTTPError`. `CaughtError.Kind` (handle_error.go) collapses to "app" (already
an `*HTTPError`, whoever built it) and "unknown" (anything else) rather than
inventing a distinction the rest of the codebase doesn't make. The merge
behaviour is unaffected — both non-unknown kit kinds share the same fallback
rule "keep the error's own status and message" — only the string a hook
branches on is coarser than kit's. Worth a real type distinction if a future
issue needs framework vs. app in the hook input; not invented here.

## Two related gaps deliberately left open

- **The mid-render `transformError` path** (a Svelte component itself
  throwing, boundary-caught — `/error/command`, `/error/boundary`) still runs
  the JS-local `handle_error` in `internal/adapter/skgo-adapter.js`, unchanged.
  Consulting the Go hook there needs a new host binding
  (`__skgo_handle_error`-shaped, alongside `__skgo_remote`), which risks
  touching `Engine.Render`'s signature at the same time #62 is adding
  `event.fetch`/`$app/paths` bindings there. Scoped out; the new e2e test in
  `example/server_test.go` (`deliberateFailures[...].handled`) pins that these
  routes do *not* carry the hook's support id, so the boundary stays visible
  rather than silently drifting.
- **`__data.json` and dev mode** (`data.go`'s `runBranchWith`/`writeNodes`)
  never consult the hook either — kit calls `handle_error_and_jsonify` there
  too (`data_serializer.js`), but it's a separate wire and a separate change.
  `dataNode` gained a `raw error` field carrying the pre-`asHTTPError` error
  for this reason, unused outside the page-render path today.
  `dev.feature`'s `/error/unexpected` scenario still expects "Internal Error"
  and is correct: dev mode never reaches Go's SSR at all (`ssr = !dev`), so
  what it exercises is kit's own *client*-side error handling
  (`HandleClientError`, a different hook entirely, declared in
  `hooks.client.ts` if at all) — not a regression, not the same hook.

## svelte-check needs `App.Error` augmented

Adding `page.error?.supportId` to `+error.svelte` broke
`TestChangingAGoTypeBreaksTheComponentThatUsesIt`: kit's default `App.Error`
is `{status, message}` and nothing else, so a field the hook adds without a
matching ambient declaration type-checks against nothing. There was no
`example/web/src/app.d.ts` at all before this. Added one declaring
`interface Error { supportId?: string }` — the standard kit convention
(`ambient.d.ts`'s own doc comment), not a workaround.
