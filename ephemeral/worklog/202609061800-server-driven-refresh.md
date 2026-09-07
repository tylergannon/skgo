# server-driven refresh (issue #4, second half) — corrections and traps

## skgo runs the client's `refreshes` list; kit refuses to

This is the largest finding of the sprint and it is **not fixed here**.

Kit has two channels and they are not the same thing:

- **A, server-driven.** `getTodo(id).refresh()` / `.set(v)` inside a command or
  form registers a thunk in `state.remote.explicit`
  (`runtime/app/server/remote/query.js:402`), drained after the handler body by
  `collect_remote_data` (`runtime/server/remote-functions.js:361`). This is what
  `skgo.Refresh` now is.
- **B, client-requested.** `.updates(...)` sends `refreshes: string[]`, which the
  server parses into `state.remote.requested`
  (`runtime/server/remote-functions.js:495`). Grep the whole kit tree: that map
  is read in exactly **one** place, `runtime/app/server/remote/requested.js:123`,
  inside `requested(fn, limit)`. Nothing refreshes until the handler calls it,
  naming the query it is willing to refresh and a maximum number of instances.

skgo's `resolveRefreshes` runs every key the client posts, with no gate. Kit's
own design note (`documentation/docs/20-core-concepts/60-remote-functions.md`,
around line 1123) says why it does not:

> **Denial-of-service.** Any malicious user can inspect their network tab to
> discover which queries your app uses, then POST a command with a
> client-supplied list of thousands of refreshes. The only defence is for the
> server handler to declare which queries it is willing to refresh — and in what
> quantity (hence the required `limit`).

`requested()` also re-validates each client payload against the query's own
schema and *fails* the over-limit ones into `data.q[key] = {e}` rather than
dropping them silently, so the client's queries visibly error instead of going
quietly stale. None of that exists here. Closing the gap needs a Go analogue of
`requested()` and a change to the example's `AddTodo.svelte` /
`TodoDetail.svelte` handlers, which is a second feature and a second set of
scenarios; it was left alone deliberately so the existing `.updates(...)`
scenarios stayed as they were.

## kit's refresh is a thunk, not a value — and the deferral is observable

`refresh(event, state, internals, payload, fn)` stores `fn`. It runs at the end
of the request. Kit's own comment says why: *"so that it observes any state
mutations that happen after `refresh()` is called."* A Go port that ran the
query at the point of the call would publish a value from the middle of the
command. `TestRefreshRunsAfterTheHandlerBody` pins it.

The drain is re-entrant (`drain()` re-enters as each promise settles) because a
refreshed query may register further refreshes. skgo's loop does the same with a
`drained` set.

## Dropping a duplicate key at merge time is too late

`collectRefreshes` first drained the handler's own refreshes and then merged the
client's list, skipping keys already present. That produced the right response
and ran the query **twice** — the second result was computed and thrown away.
For a query with side effects or cost that is a real bug, and the merge looked
correct in the response body, which is where a test would have looked. Caught by
counting invocations, not by comparing keys. The client's list is now filtered
*before* `resolveRefreshes` runs.

## `skgo.None` must encode to the empty payload, not to `{}`

Kit's client calls a no-argument query with `undefined`, and
`stringify_remote_arg(undefined)` returns `''` — so the cache key ends in a bare
trailing slash, `<hash>/getTodos/`. `encodeValue(None{})` produces `{}`, whose
payload is a non-empty base64 string. A refresh keyed that way is not an error
anywhere: kit's client parks an unknown key in `query_responses` and moves on
(`runtime/client/remote-functions/shared.svelte.js`), so the page simply never
updates and nothing reports anything. Special-cased in `queryPayload`, pinned by
a golden.

## A stale server on the port answered an entire suite

`/todos/pair` rendered the `[id]` route showing *No todo with id "pair"*, which
reads exactly like a kit route-priority bug — the built manifest had the routes
in the right order. The real cause: `pkill -f "example/cmd -listen ..."` matches
nothing, because `go run` executes a compiled binary out of a temp directory and
that string is not in its command line. The new server logged
`bind: address already in use` and exited; the *previous* build kept serving.
Two features went red against code that was never running.

Kill by pid from `lsof -nP -iTCP:<port> -sTCP:LISTEN`, and read the server log
after starting one — it says which pid is listening, and it says when it failed.

## Function-pointer identity is exact here because the generator makes it so

`skgo.Refresh(ctx, getTodo, id)` finds the registration by
`reflect.ValueOf(fn).Pointer()`. `reflect`'s docs disclaim uniqueness, and that
disclaimer is real for method values and for two instances of one closure — but
`readMarker` (`internal/gen/scan.go:367`) refuses anything but an `*ast.Ident`
naming a function declared in the same package, so only named top-level
functions are ever registered, and Go gives two of those distinct code pointers
even when their bodies are identical (checked). `NewRemotes` refuses a registry
where two registrations share a pointer, so the case the disclaimer warns about
is a startup failure rather than a refresh that updates the wrong query.

## Regenerating the payload goldens

The recipe in `202609060830-codec-conformance.md` still works, with exact line
ranges for the pinned kit: from
`reference/kit/packages/kit/src/runtime/shared.js` take lines 42-173
(`is_plain_object` through `create_remote_arg_reducers`), 246-259
(`stringify_remote_arg`), 306-315 (`url_friendly_base64_encode`) and 333-339
(`create_remote_key`); prepend `encoders = {}`, a `base64_encode` over
`Buffer`, a `TextEncoder`, and an import of
`reference/devalue/index.js`; strip the `export` keywords. Run it with
`mise x -- node` from `example/`. Slicing on function boundaries matters — an
off-by-one lands mid-function and node reports only `Unexpected end of input`.

## The Svelte MCP server is not loaded for this worktree

`.claude/worktrees/<branch>` is a different project directory from
`/Users/tyler/src/skgo`, and the Svelte MCP server is configured per directory,
so `svelte-autofixer` and `list-sections` were unavailable. The two new
components are copies of the patterns already in `example/web/src` and were
validated by running the app, but a session that needs to write novel Svelte
here has to fix the MCP configuration first.
