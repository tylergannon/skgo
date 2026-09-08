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
