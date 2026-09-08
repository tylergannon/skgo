# The dev document's styles (#40, mission 2)

## Kit's generated dev tree has a client half too, and nothing was writing it

Mission 3 found that kit writes `<outDir>/generated/dev` lazily, from
`update_manifest`, which only runs when kit's *own* dev middleware answers a
request — and under skgo it never does. It fixed the server half by calling
kit's `write_server` directly. The client half is written by the same
`sync.create` line and nobody was calling it, so on a tree that has never run a
bare `vite dev` the boot script's `client.app`
(`generated/dev/client/app.js`) is a 404 and the page never hydrates.

`gojaDevEnvironment` now calls kit's `write_client_manifest` beside its
`write_server`, over the same `create_manifest_data` result — once, at startup.
Not on a route change: kit's own route watcher is not gated on a request the way
`init_manifest` is, so from the first added or deleted route onwards kit keeps
that tree current itself. Writing it a second time only makes vite reload the
page for a file that already says what it says, which was enough to make
mission 3's added-route scenario fail on a screenshot taken mid-reload.

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
