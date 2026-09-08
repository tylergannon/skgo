# The front page as an index, and the two fixtures #63 named

## A Go load in the root layout makes every page in the app un-prerenderable

This is the trade the mission did not know it was making, and it is not
recoverable inside the example app.

#63's first fixture — a load that fails in the root layout takes kit's static
`error.html` — can only be built one way. Kit's `create_manifest_data` always
synthesises a root error node (`root.error.component ??= fallback/error.svelte`),
so `route.errors[0]` is defined for every route in every app, and skgo's
`nearestErrorPages` therefore finds a candidate for any failing branch index
above 0. The static page is reachable from a load failure at node 0 and nowhere
else, and node 0 is the root layout. A route group with its own
`+layout.server.ts` does not work; nor does deleting `src/routes/+error.svelte`.

But kit runs a branch's server loads while it prerenders, and skgo's are
generated stubs that throw. So a root `+layout.server.ts` fails the build of
every `prerender = true` page in the app:

    Error: skgo: implemented in Go
        at load (src/routes/+layout.server.ts:10:51)
    500 GET /about — Failed to prerender /about

`/about` was the only prerendered page, so this PR gives it up, along with
`TestEveryPrerenderedPathIsServedFromItsFile` and routes.feature's prerender
scenario. `static_test.go` still covers prerendered serving against a synthetic
dist, so the library's coverage is intact; what is gone is the end-to-end proof
that kit's build and skgo's server agree about it, and one row of the index.

The real fix is a skgo feature nobody has scoped: answering a Go server load
while kit prerenders. There is no Go process in `vp build`, so it would mean
either the adapter shelling out to the app's binary or skgo doing its own
prerender pass after the build. Until then an app chooses one or the other, and
`about/+page.ts` says so where the next person will read it.

## A root layout server load changes the data traffic of every route

Kit's client asks for `__data.json` on a navigation only if some node in the
branch has server data. Give the root layout a load and that becomes every
route, dev included — where the app turns SSR off, so the *first* page load
fetches it too. stream.feature marked the data-request count immediately after
`page.goto('/about')` and then counted `/about`'s own request against the
navigation it was measuring. The fix is to assert the page has arrived before
marking; the scenario now waits on the root layout's own value.

Worth checking in any scenario that counts data requests across a navigation.

## `just serve`'s pid is the pipeline's, not the server's

`SKGO_PORT=... just serve &` gives `$!` for `sh -c 'go run ... | tee'`. Killing
that leaves the compiled binary holding the port. Every later `just serve` then
fails to bind — into a log nobody reads — while the *old* build keeps answering,
so a rebuilt page appears not to have changed and the suite fails on content
that is correct in the source. Kill what `lsof -nP -iTCP:<port> -sTCP:LISTEN -t`
reports, and confirm the new server is the one answering by curling something
only the new build says.

## The Svelte MCP server is not loaded in every session

`/Users/tyler/src/CLAUDE.md` requires `svelte-autofixer` on any component
written. It was not among this session's tools. `mise x -- pnpm check` in
`example/web` (svelte-kit sync + svelte-check) is the fallback that actually
type-checks the app; it caught nothing here, but `vp build` caught an apostrophe
inside a single-quoted string that a global smart-quote replacement had
introduced.
