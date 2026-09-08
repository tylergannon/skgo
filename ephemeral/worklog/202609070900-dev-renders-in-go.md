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
