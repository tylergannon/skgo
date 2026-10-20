# Svelte-only route add in `skgo dev`

- Cause, measured 2026-10-19 on v0.41.1 (fresh `skgo new`, outside-in: write
  `+page.svelte`, poll until 200 with the fixture token): add took a median of
  2.79 s, and every add and delete rebuilt the Go host. The cause was the generated
  `<out>/params/skgo_gen.go`, which listed every Kit route by ID (one
  `SkgoParams` case per route and the `SkgoCallerRoutes()` table). Any file
  hotreload cannot install makes `hotreload.Host.Prepare` return
  `CompleteBuild`.
- Fix: the shared params constructor is keyed by the declared (name, matcher)
  pairs Kit already supplies at runtime: manifest `ManifestRoute.Params`, the
  dev external match `RouteParams`, and `handleRoute`/`dataRoute.params`.
  `CallerParams` replaces `CallerRoutes`. Drift is now "a declaration with no
  generated variant", checked with nil values. Optional, Rest and Chained flags do not
  affect types, so they are not part of the key.
- After, same machine: median add is about 0.14 s, with zero rebuilds and the
  same Go pid. A Svelte-only route reusing an existing `[slug]` takes 0.37 s
  with no rebuild. A new declaration or a new Go file still rebuilds, as it
  must.
- Under a root `layout.server.go`, a Svelte-only child also needs no rebuild
  (0.13–0.23 s). The layout's generated code does not depend on that child.
- Trap: the dev native path matches through middleware's external/prerender
  branch (`build.RouteID`/`build.RouteParams`), not the manifest route
  matcher. Hook state must take declarations there as well.
- `plugins/native-shell/example` pins the skgo release (v0.41.0, no replace).
  Its generated code tracks that pin and is regenerated only when it bumps skgo.
