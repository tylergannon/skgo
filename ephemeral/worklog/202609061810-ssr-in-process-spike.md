# SSR inside the Go process — feasibility spike

Branch `claude/ssr-no-node-sidecar-9e3be4`. Not mergeable as-is; see "Landmines".

## Landmines this branch leaves behind

- `example/web/src/routes/+layout.ts` now says `export const ssr = true`. With
  `ssr = false` kit omits `component` from `.svelte-kit/output/server/nodes/N.js`
  entirely, so there is nothing to render and the whole spike has no input.
  Anything that reverts it must also stop expecting the spike to rebuild.
- `example/web/src/routes/ssr-probe/+page.svelte` is a route added for the spike.
  It exists only because no page in the app can server-render a remote function
  (see below). It has no Go counterpart and shifts every node index above 12.
- `go.mod` gained goja (and quickjs-go, used only under `-tags quickjs`).

## The finding that decides the shape of any real SSR work

**Svelte drops the children of a `<svelte:boundary>` that has a `pending`
snippet from the server output entirely.**
`svelte/src/compiler/phases/3-transform/server/visitors/SvelteBoundary.js:59-74`:
when a `pending` snippet or attribute is present, `children_body` *becomes* the
pending block. The children are never emitted. Kit's own build of the example
app proves it — `.svelte-kit/output/server/entries/pages/_page.svelte.js` for
the home page contains the `loading…` paragraph and nothing else.

Every await in the example app sits inside such a boundary, because the app was
written for `ssr = false`. So turning SSR on changes nothing about what the app
shows until its components change: the shape of the markup, not the server, is
what decides whether a page is server-rendered. Any SSR sprint has to plan for
rewriting the app's boundaries, and any acceptance scenario that asserts
"the page arrives with its data in the HTML" will fail on the app as it stands
today for reasons that have nothing to do with Go.

## Async SSR needs AsyncLocalStorage, and there are two ways to not have it

`svelte/src/internal/server/render-context.js:49` throws
`async_local_storage_unavailable` if `als === null` — proven, `als=none` variant.
Two things make it work in a bare engine, and they produce byte-identical output
(`TestWebcontainerVariantMatches`):

1. `globalThis.process = { versions: { webcontainer: '…' } }` — svelte's *own*
   fallback (`render-context.js:39,83`): a module-global context plus a
   serialised render queue. Costs one line of banner and no shim. It also flips
   kit's `IN_WEBCONTAINER` (`kit/src/constants.js:30`), which stops
   `with_request_store` nulling its synchronous store (`event.js:82`).
2. A hand-written `AsyncLocalStorage`. Two non-obvious requirements, both found
   by it failing:
   - **the slot must be per instance.** svelte and kit each construct their own
     `AsyncLocalStorage`. A module-level slot lets svelte's
     `als.run(render_context, …)` overwrite kit's store, and
     `get_request_store()` then returns `{event: undefined, state: undefined}` —
     surfacing as `Cannot read properties of undefined (reading 'remote')`
     inside `get_cache`.
   - **`run` must not restore the previous store.** `als.run(store, asyncFn)`
     returns at the first `await`.

Prefer (1). It is svelte's supported path and there is nothing to maintain.

Either way the request store leaks past the end of a render, holding the derived
remote-function store with `is_in_remote_query: true`
(`__skgo_leaked_store()`, logged by `TestAsyncLocalStorageVariants`). Harmless
between sequential renders, because each render opens with `with_request_store`,
which overwrites it. Not harmless if two renders overlap.

## `als` is not installed until the job queue has been drained once

Both kit (`event.js:17`) and svelte (`render-context.js:73`) assign their
`AsyncLocalStorage` from a `.then()` on `import('node:async_hooks')`. Evaluating
the bundle is not enough — the host has to let microtasks run before the first
render, or the whole thing silently falls back to the synchronous store and then
throws "Could not get the request store. This is an internal error." in the
middle of the first async component. In goja, one no-op `RunString` after
`RunProgram` does it.

## goja parses the bundle only with three features turned off

esbuild `supported: { 'dynamic-import': false, 'import-meta': false,
'for-await': false, 'async-generator': false }`. `for await` (kit's live-query
and streaming paths) and async generators are the two the earlier probe did not
hit. `target` must stay at `es2022`: downlevelling private class fields breaks
svelte's server `Renderer`.

## One Runtime renders one page. A nested render vanishes without an error.

`TestReentrantRenderOnOneRuntime`: a Go host function is a re-entry point. Start
a second render from inside one, and goja returns from it with the promise still
pending — because the job queue is only drained when the *outermost* call
returns. Go gets `{done:false, body:""}` and **no error**. The outer render is
unharmed. A pool of Runtimes, one render each, is the only safe shape; anything
that reads a render result must check `done` rather than trusting the absence of
an error.

## Don't read `manifest.js` for node indices

`.svelte-kit/output/server/manifest.js` is prerender-trimmed: it drops nodes and
**renumbers the array**, so its `leaf`/`layouts` indices do not line up with
`nodes/N.js` on disk. `manifest-full.js` is the one that does.

## Reversing kit's route-directory encoding is a guess; the sourcemap is not

`entries/pages/items/_id_/_page.svelte.js` → `src/routes/items/[id]/+page.svelte`
is recoverable exactly, from the `sources` array of the `.map` kit emitted
beside it. Hand-written `_` ↔ `[` rules break on `[...rest]`.
