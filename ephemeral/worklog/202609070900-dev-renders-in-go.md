# Dev renders in Go (issue #40, mission 1)

## Traps confirmed or found

- **`example/e2e/shot.mjs` and `example/devspike/` are the untracked spike.** They
  are deleted before the PR; nothing spike-named ships.
- **The dev and build node numbering agree for the example app**, checked by
  reading `.svelte-kit/generated/dev/client/nodes/*.js` against the built
  manifest's node table: 32 nodes, same order, same components. That is luck of
  this app (nothing is prerendered out of the table), not a rule — mission 3
  owns making Go's dev route table come from vite.
- **`hotUpdate`, not the raw watcher, is the invalidation feed.** Vite's
  `onFileChange` invalidates every environment's module graph *before* it calls
  `handleHMRUpdate`, and `handleHMRUpdate` is what dispatches `hotUpdate` to a
  plugin. A listener on `server.watcher` races the invalidation and Go would
  re-fetch the module it already had.
- **`applyToEnvironment` does not hide `hotUpdate` from the goja environment.**
  `handleHMRUpdate` loops over every non-client environment and asks
  `getSortedHotUpdatePlugins(environment)`, so the plugin gets exactly its own
  environment's invalidations.

## Traps the run found

- **A rendered document is clickable before it is live, and in dev that window is
  seconds wide.** The suite lost a `fill()` (Svelte's `bind:value` writes the
  component's empty state over what was typed the moment it hydrates), lost
  clicks entirely, and counted a live-query stream opening as the scenario's own
  interaction. Prod's bundle closes the window, which is why nobody had seen it.
  `steps/fixtures.ts` `hydrated()` waits on `history.scrollRestoration ===
  'manual'` — kit's own `_start_router` sets it, and `_start_router` runs after
  `_hydrate` resolves.
- **Vite's HMR socket dials the page's own origin at `/`.** That is the app's
  home route, and answering it as a document gives the browser a 200 and HTML,
  which it reports as a failed handshake before falling back to talking to vite
  directly. An upgrade is never a document, whatever path it names.
- **A virtual module is invalidated by nothing.** The node table is generated,
  not read from a file, so adding a route left the engine holding the old
  numbering and rendering whichever components lived at those indices — with the
  right route matched and the wrong page rendered. The dev plugin watches kit's
  generated node directory and names the table's own URL in the change log.
- **`go run` is the wrong way to hold a port.** Killing the `go run` pid leaves
  its child listening. Build to a file and run that, so the pid is the server.

## Which scenarios are load-bearing, measured

Two mutations, each run against a real `just dev`, whole `dev.feature`:

- **Go forwards the page GET to vite again** (`devPages.ServeHTTP` never calls
  `renderer.serve`) — the four document scenarios fail (edit-reaches-bytes,
  server-only failure, Go query in the document, Go load in the document); the
  five endpoint scenarios still pass, because `__data.json` and the remote
  endpoints are Go's either way.
- **`Runner.Refresh` stops invalidating** (returns before `invalidate`) — only
  the edit scenario fails, on its 30 s poll. Everything else is unaffected,
  which is what makes that scenario the one that speaks for hot updates.

So no single scenario covers both mechanisms, and neither is redundant.

## Rebase onto #95 (`@skgo/adapter` as a package)

- **The dev environment needs the build environment's app-side resolution.**
  #95 moved the adapter out of the app and into `node_modules/@skgo/adapter`, so
  `entry.js` no longer sits inside the vite root and its bare specifiers
  (`@sveltejs/kit/internal/server`, `svelte/server`) resolve against a
  `node_modules` that is not the app's. The build half already answers this by
  re-asking through `this.resolve(id, join(root, 'package.json'))` for an
  importer of ours; the dev half's `resolveId` had to grow the same branch, and
  `fetchModule` has to come from the app's vite for the same module-realm reason
  rolldown does.
- **Svelte's dev compile keeps the scoping class the build's compile prunes.**
  After #94 gave `+page.svelte` a `<style>` block, the served document says
  `<h1 data-testid="title" class="svelte-1uha8ag">` where the built one says
  `<h1 data-testid="title">`. Nothing to do with skgo — kit's own dev server
  renders the same markup — but any scenario asserting a whole tag as a literal
  is a scenario that means one thing in prod and another in dev. `tagged()` in
  `steps/ssr.ts` is the shape that survives both, and mission 4 will want it
  everywhere a `<tag data-testid=...>` literal is asserted today.
- **Editing the adapter invalidates the built frontend.** The stamp Go checks is
  a fingerprint over the adapter's own files, so every edit under
  `internal/adapter/` needs `just build` again before the server will start.

## One flake, unreproduced

`dev.feature`'s "A page marked ssr = false is the same shell it is in prod"
failed once in five full dev runs and passed in every other, including a run
against a cold `vp dev` with `node_modules/.vite` deleted and a run of that
scenario alone. Nothing about it is skgo's dev renderer — Go declines that
branch and vite answers it — so it is either kit's dev shell arriving late or
vite reloading the page under the step. Worth a `page.on('framenavigated')`
trace next time it shows up rather than a blind timeout bump.
