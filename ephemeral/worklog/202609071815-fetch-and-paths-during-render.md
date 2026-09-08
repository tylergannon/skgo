# event.fetch and $app/paths during render (#62)

## `.svelte` files cannot import `$app/server` — a real kit rule, not a skgo gap

The issue's proof asked for "a page that fetches a `+server.ts` route during
render." That page cannot exist: kit's `vite-plugin-sveltekit-guard`
(`exports/vite/plugins/guard.js`) refuses to build a `.svelte` file that
imports `$app/server`, because every route's `.component` is unconditionally a
client entrypoint — regardless of `ssr`/`csr` — and `getRequestEvent()`/
`event.fetch` are only ever sanctioned inside hooks, loads, actions, endpoints,
and remote-function bodies (`getRequestEvent`'s own JSDoc, and confirmed by an
actual build failure: "Cannot import $app/server into code that runs in the
browser"). skgo implements all four of those exclusively in Go, and its
`$app/server` substitution discards a `.remote.ts`'s user-authored body before
ever calling it — so there is no application code path, in this architecture,
where JS legitimately calls `event.fetch`. This is true of any kit app, not
specific to skgo.

Given that, the render-time fetch host binding is implemented and proven at
two levels instead of a live page: `document_fetch_test.go` (Go-side dispatch
against a real `http.Handler`) and `internal/ssr/ssr_test.go`'s
`TestFetchIsACallBackIntoGoNeverASocket` (a synthetic bundle that calls
`__skgo_fetch` the same way the real polyfill does, bypassing kit's build
entirely). Both exist so a *future* legitimate call site (a universal load,
say) has a working, tested mechanism to reach. There is no Gherkin scenario for
fetch and no example page for it; a page that pretends to call `event.fetch`
would not build.

## `resolve`/`asset` already worked; only `match` was actually broken

The parent issue claimed all three of `resolve`, `asset` and `match` were
"not exercised." Reading kit's source: `resolve`/`asset`
(`runtime/app/paths/server.js`) are pure string logic over the app's
compiled-in `base`/`assets` constants and need nothing this engine lacks — the
alias already pointed at kit's real file before this issue, and it already
worked, just untested. Only `match` was genuinely broken: it needs
`manifest._.matchers()`/`find_route`, and skgo's entry never calls kit's
`set_manifest()`, so `manifest` is `null` and any call throws.

Fix: `$app/paths` is aliased to a new `skgo:app-paths` wrapper that re-exports
kit's real `resolve`/`asset` unchanged and replaces only `match` with a
version that mirrors kit's own preparation (decode the pathname, strip the
base) and then asks Go — `Loads.match`, the exact route table a real
`+server.ts`/page/`__data.json` request already matches against — instead of
building a second router for the engine.

## Client-side `resolve`/`asset` legitimately disagree with the SSR document

`runtime/app/paths/client.js`'s own doc comment: "During server rendering, the
base path is relative and depends on the page currently being rendered" — the
client version is *always* absolute (`base + ...`), never relative. So a
screenshot of `/render-paths` taken after hydration shows `/api/todos` where
the SSR document (what the Gherkin scenario actually asserts on, via
`documents.last`) said `./api/todos`. Not a bug; documented in
`ssr.feature` so nobody "fixes" it later.

`match()`'s client implementation also legitimately differs: the client
manifest only carries page-navigable routes, so `match('/api/todos')` (an
endpoint-only route) returns `null` client-side even though Go's route table
(which includes endpoints) finds it. The demo page targets `/items/77` (a page
route) for `match` specifically to avoid a screenshot that looks like a
failure when it isn't one.

## Mutation checks

- `internal/ssr`: commented out the `__skgo_fetch` registration in
  `newRuntime` → `TestFetchIsACallBackIntoGoNeverASocket` failed
  ("TypeError: Object has no member '__skgo_fetch'"). Restored → passed.
- `document.go`: set `Fetch`/`Match` to `nil` in `renderPlan`'s
  `ssr.Hosts{...}` → e2e scenario "match finds the same route id a real
  request to it resolves" failed (`match('/items/77') = null`); "resolve and
  asset produce the relative hrefs..." still passed, correctly, since
  `resolve`/`asset` never needed a new binding. Restored → both passed, full
  74-scenario prod suite green.
