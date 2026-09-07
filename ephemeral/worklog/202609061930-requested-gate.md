# the requested-refresh gate (issue #33) — corrections and traps

Closes the finding recorded in `202609061800-server-driven-refresh.md`.

## A cross-package refresh cannot be spelled without the link tree

The gate makes a handler *name* the query, and that turns skgo's module layout
into an ergonomics problem the first time a command in `src/lib` has to accept a
refresh of a query in a route. `src/routes` is its own Go module — skgo writes
the boundary `go.mod` there because `[id]` is not a legal import path segment —
so `github.com/…/example/web/src/routes/todos` is not importable from the app
module at all. The only address is the generated link:

    todos "github.com/tylergannon/skgo/example/generated/links/onzggl3sn52xizltf52g6zdpom"

(base32 of `src/routes/todos`; it is in `example/generated/links.json`.) It
compiles, it is stable, and it is what `example/web/src/lib/auth.remote.go` now
does — but it reads badly, and it is the one place in the example where an
authored file imports a generated path. Two things follow:

- Do **not** try `.../web/src/routes/todos` instead. It is outside the app
  module; if the boundary ever went away it would compile as a *second* copy of
  the package with different code pointers, and `lookupFunc` identifies a query
  by its code pointer. The failure would be "not a registered remote function"
  on a function that is plainly registered.
- The route package should decide what it lets others refresh, not the caller:
  `todos.AcceptSessionRefreshes(ctx)` keeps `getTodos`/`watchCount` unexported
  and gives `lib` one function to call. If a friendlier alias for a route
  package is ever generated, that call site is the thing to point it at.

## `every part of the page loaded` counts `*-failed`, so a deliberate error must
not be named one

A page that *should* show an error — the gate page's refused panel — must not
put it behind a `data-testid` ending in `-failed` or `-pending`. `steps/app.ts`
asserts both are zero on every page, so an intended refusal would fail an
unrelated step. `note-refused-<id>` is the naming used instead.

Related: don't render a deliberately-failed query inside `<svelte:boundary>`.
The boundary latches until it is reset, so the panel could not recover on the
next refresh. Read `q.error` / `q.current` directly; both getters call `start()`,
and `refresh()` clears `#error` on success
(`runtime/client/remote-functions/query/instance.svelte.js`).

## A mutation that "removes the gate" can leave a scenario green for the wrong
reason

Restoring the old ungated behaviour by pre-registering every posted key in
`newRefreshSet` did **not** turn the over-limit scenario red: the handler's own
`RefreshRequested` still ran afterwards and its refusal overwrote the
pre-registered entry under the same key. The break that proves that scenario is
the one aimed at the limit itself (`if i >= limit` → never). Three separate
breaks were needed for three scenarios; one blunt one would have signed off a
scenario nothing was testing.

## `EXPECTED_MODE` is not a free-form label

Passing `just e2e break1` to tag a mutation run turns seven unrelated scenarios
red: `the document response came from skgo in the expected mode` compares
`X-Skgo-Mode` against it. Use `prod`/`dev` and tell the runs apart some other
way — the stray value also litters `ephemeral/screenshots/<feature>/<mode>/`.
