# The dev document's styles (#40, mission 2)

## Kit writes its generated dev tree lazily, and under skgo nothing triggers it

`sync.create` — which writes `<outDir>/generated/dev/client/app.js` and
`nodes/*.js` — runs from `update_manifest`, and `update_manifest` runs from
`init_manifest ??= update_manifest()` inside kit's *own* dev middleware
(`exports/vite/dev/index.js`). That middleware is last in the stack, so it only
ever sees a request nothing else answered. Under `just dev` Go answers the
documents and vite's own middlewares answer the modules, so kit's never runs:
on a tree that has not had a bare `vite dev` in it, the node table is not on
disk and the engine dies at boot, and the boot script's `client.app` would 404
even if it did not. This is not a race; it never happens.

`gojaDevEnvironment` now makes one request to the dev server before it answers
`/__skgo_dev/info`, which is the only way to reach that middleware from inside
the plugin. Kit's route watcher keeps the tree current afterwards, so it is a
cold-start warm-up and nothing more.

## Editing the adapter means rebuilding before `just dev`

Anything under `internal/adapter/skgo-adapter/` changes the adapter hash the
manifest carries, and Go refuses a build whose adapter is not its own. So every
`env.js` edit costs an `ORIGIN=... just build` before the dev server will come
up, even though dev evaluates none of that build's bundle.

## A dev plugin's `config()` `server` key is ignored under `vp dev`

Two attempted mutations went through it — `server.hmr.clientPort` and
`server.watch.ignored` — and both were silent no-ops: the suite stayed green and
HMR kept working. Whatever merges plugin config here, `server` is not part of
it. A `hotUpdate` hook does take effect, and returning `[]` from one that
applies to every environment is the mutation that kills HMR in the browser
while leaving Go's own invalidation cursor alone.

## Playwright's `browser` fixture inherits the project's context options

`browser.newContext()` inside the `noscript` project produces a context with
`javaScriptEnabled: false`, because Playwright applies the test's own
`contextOptions`. A helper context that has to run script — signing in, say —
must ask for `javaScriptEnabled: true` outright. The symptom is a silent one:
the helper page loads, renders, and does nothing, with an empty console.
