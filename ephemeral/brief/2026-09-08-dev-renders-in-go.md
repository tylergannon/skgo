# Dev renders in Go — issue #40

Written 2026-09-08 against kit `3.0.0-next.25`, vite-plus `0.3.0` (vite 8) and
PR #80. Line anchors are `/Users/tyler/src/skgo/ephemeral/inspiration/reference/`.
Everything claimed as observed was run in the worktree
`.claude/worktrees/agent-af0cac6a1631c947e`.

## The answer

**The `goja` environment PR #80 declares for the build should also exist in
`vite dev`, and Go should run Vite's module runner inside goja against it.** No
bundle, no second renderer, no Node at render time. Go asks the dev server for
one transformed module at a time over `fetchModule()`, evaluates it in the same
engine the built bundle runs in, and drops exactly the modules an edit
invalidated. `export const ssr = !dev` then has nothing left to work around.

This was built and run. Go rendered the example app's home page from 203 modules
served by `vp dev`, with the value of the `getSite` query in the markup, in
440 ms cold; a component edit re-fetched and re-evaluated **3** modules in
**5 ms** and the second render carried the edit.

## Kit facts

- **`vite dev` never runs an adapter.** `adapt()` is reached only from
  `finalise`, and the plugin that calls it is `apply: 'build'`
  (`kit/src/exports/vite/index.js:1591`, `:1691`). So a dev design that needs a
  built SSR bundle needs something other than kit to build it, and needs to
  invent the node table `builder` would have supplied.
- **Kit still hands the adapter its dev plugins.** `adapter.vite.plugins.pre`
  and `.post` are members of the plugin array kit returns unconditionally
  (`:1706`, `:1735`) — not gated on build. An adapter can declare a Vite
  environment in dev without kit being patched, wrapped or forked.
- **Kit's dev server renders through `runner.import`.** It is a *Node* module
  runner: `get_runner` asserts `isRunnableDevEnvironment(server.environments.ssr)`
  and returns that environment's runner (`kit/src/runner.js:9-14`), and every
  node, hook, endpoint and remote module is loaded through it
  (`exports/vite/dev/index.js:90`, `:112`, `:317`, `:346`). This is what calls
  skgo's throwing stubs today.
- **A server load has no wrapper to alias.** `load_data.js:34` is
  `const load = node.server.load;` — kit calls the module's own export. A
  dev-only `$app/server` covers `query`/`command`/`form` and nothing else, which
  is what `ephemeral/worklog/202609062230-dev-is-csr.md` found and it is still
  true.
- **The dev document differs from the built one in four places.** Kit's dev
  manifest gives `client.start` = kit's own `runtime/client/entry.js` and
  `client.app` = `<outDir>/generated/dev/client/app.js`, with empty `imports`,
  `stylesheets` and `fonts` (`exports/vite/dev/index.js:192-199`); per node it
  supplies `inline_styles()`, which walks vite's module graph and re-imports
  each CSS dep with `?inline` to avoid FOUC (`:279-297`); the style tag it
  produces carries a `data-sveltekit` attribute so CSR can remove it
  (`runtime/server/page/render.js:302`); and the boot global is
  `__sveltekit_dev` rather than `__sveltekit_<hash>` (observed).
- **HMR does not belong to whoever renders the document.** Kit's client runtime
  contains a bare `import.meta.hot;` whose only purpose is to make vite inject
  its dev client as that module's first dependency
  (`runtime/client/payload.js:15-17`). Fetched from the running server, that
  module really does begin
  `import { createHotContext as __vite__createHotContext } from "/@vite/client"`.
  So any document whose boot script imports the client entry from the dev server
  gets HMR, whether Go or kit wrote the document.
- **`__SVELTEKIT_DEV__` is a compile-time define, not a runtime flag**
  (`core/env.js:53`, `exports/vite/index.js:493`), so it is already true in every
  environment of a dev config, including a new one.
- **Kit's remote plugin applies to any server environment and does its epilogue
  in dev too** (`exports/vite/index.js:609-611`, `:656`, `:673`). In dev it
  appends `await Promise.resolve()` — real top-level await — and assigns the same
  `<hash>/<name>` ids the build assigns. Those are the ids Go already answers.
- **A non-literal page option makes kit's static analysis return null**
  (`exports/vite/static_analysis/index.js:179-191`), which is why `ssr = !dev`
  works at all today and why deleting it changes nothing structural.

## Vite facts

- `fetchModule(environment, url, importer, options)` is exported from vite
  (`vite/src/node/ssr/fetchModule.ts:24`) and works on any `DevEnvironment`,
  runnable or not. It returns `{code, file, id, url, invalidate}` — the SSR
  transform's output, wrapped by the caller.
- The evaluation contract is six names:
  `__vite_ssr_exports__`, `__vite_ssr_import_meta__`, `__vite_ssr_import__`,
  `__vite_ssr_dynamic_import__`, `__vite_ssr_exportAll__`,
  `__vite_ssr_exportName__` (`vite/src/module-runner/constants.ts:2-7`), and the
  body is wrapped in an **async** function
  (`vite/src/module-runner/esmEvaluator.ts`). goja parses async functions; it
  does not parse `for await` or async generators, which is what PR #80's
  per-module lowering already exists for.
- `resolve.noExternal: true` on the environment is what keeps everything inside
  the transform: import analysis only leaves a bare specifier alone when
  `shouldExternalize` says so (`plugins/importAnalysis.ts:556-560`,
  `external.ts:115-117`), and `fetchModule` externalizes bare specifiers before it
  ever consults a plugin (`ssr/fetchModule.ts:44-79`).
- A custom dev environment defaults to a Node `RunnableDevEnvironment`
  (`node/config.ts:271-272`); nothing forces skgo to use its runner, and
  `createFetchableDevEnvironment` exists for exactly this shape of host
  (`server/environments/fetchableEnvironments.ts`).

## The candidates

**(a) Go runs the module runner — recommended.** Cost: skgo owns a module
runner. In the spike that is a module cache, a reverse-import graph, and a loop
that resolves one pending `__vite_ssr_import__` at a time from Go with no
JavaScript on the stack — goja drains its job queue when the outermost call
returns, which is what lets a suspended module continue. It is a second
implementation of something Vite ships in JavaScript, and it will track vite's
SSR-transform contract. Against that: the contract is six identifiers and one
result shape, and everything else — `entry.js`, `app-server.js`, `app-paths.js`,
the node table, the host bindings, the polyfill, the per-module lowering — is
PR #80's, unchanged. One renderer, one set of substitutions, two module sources.

**(b) Watch-mode build of the fourth environment.** Not dead, but it pays twice.
`adapt()` never runs in dev (`index.js:1691`), so the node table, the manifest
and the fold would have to be driven by skgo outside kit's build. And every save
costs a whole-app rolldown pass where (a) costs 3 modules: the browser reloads on
vite's HMR message, which is sent before any rebuild of ours could finish, so Go
would have to block document renders behind a rebuild it does not control. The
only thing it buys is not writing a module runner.

**(c) Dev-only `$app/server` shim, kit keeps rendering.** Still dead, for the
reason the worklog gave and one more. `load_data.js:34` calls the module's own
`load` export, so there is no specifier to alias for a server load — a third of
the example app's routes would still 500. And it leaves kit rendering pages in
Node, which is the thing the owner has ruled out.

**(d) Anything kit suggests that these miss.** Kit's own answer for "another
runtime renders" is the runner protocol; PR #15574, which tried to make kit's
dev server and prerenderer run inside a foreign environment, was closed for
complexity, and `adapter.vite.plugins` (#16206) is the piece kit kept. That is
precisely the seam (a) uses. There is no other hook.

## What the spike proved

Everything below was observed, not reasoned. The spike is
`example/devspike/` (Go), `example/web/vite.spike.config.ts` (the dev
environment and one `/__skgo_fetch` middleware) and
`example/web/skgo-spike-runtime/` (PR #80's `entry.js`, `app-server.js`,
`app-paths.js` verbatim, plus a polyfill). Documents and screenshots are in
`ephemeral/spike/devrender/`.

- The `goja` environment can be declared in `vite dev` beside kit's three, and
  kit's own dev server keeps working (`/` still answers 200 with its shell).
- Its modules can be pulled one at a time and evaluated in goja: **203 modules**,
  including Svelte's server renderer, kit's `Root`, kit's remote wrappers and the
  app's `.svelte`/`.ts` sources, in **440 ms** cold.
- A render of `/` produced real markup with **the query's value in it** —
  `<p data-testid="site-name">skgo</p>` and
  `<p data-testid="colocated">src/routes/site.remote.go</p>` — answered by the
  app's own Go `getSite` through `skgo.Remotes`. The generated `.remote.ts` stub
  still throws, so the value is proof Go answered.
- A component edit invalidated **3 modules** (`+page.svelte`, the node table,
  the entry) and reloaded them in **5 ms**; the next render carried the edit.
  `rendered-before.png` and `rendered-after.png` are those two documents, opened
  and looked at: nav, sign-in form, `Home` → `Home, hot`, and the query's value.
- Kit's dev remote epilogue gives the same `<hash>/<name>` ids Go already
  serves (`ezl04l/getSite`, `4cga8b/whoami` — both answered).

## What the spike did not prove

- **No document assembly.** The spike wraps `{head, body}` in a bare shell. It
  has no boot script, no hydration data, no per-node inline styles — which is why
  the screenshots are unstyled. Whether the dev document hydrates without a
  mismatch is untested.
- **One route.** `/` only. `/todos` hangs, in the spike's own naive host: it
  answers a `query.live` by replaying `Remotes.ServeHTTP` into an
  `httptest.Recorder`, and that streams. `document.go` already solves this
  properly; the spike does not.
- **No server-load data.** Nothing was rendered with `branch[].data`. That path
  is identical in dev and prod and PR #80 proves it, but it was not run here.
- **HMR end to end.** The browser half is argued from `payload.js:15-17` and
  from the transformed module fetched off the running server. No browser was
  driven.
- **Nothing about a second app, or scale.** 203 modules is this app.

## Traps found, which will cost someone a day each

- **`esm-env`'s `DEV` must be `true` in dev, where PR #80's build says `false`.**
  `vite-plugin-svelte` compiles components with Svelte's dev instrumentation in
  serve, and `push_element` reads `ssr_context.function`, which Svelte's own
  `push()` only sets `if (DEV)`
  (`svelte/src/internal/server/context.js:63-69`,
  `internal/server/dev.js:57-59`). Saying `false` gives a component compiled one
  way a runtime built the other, and the render dies mid-tree with
  `Cannot read property 'filename' of undefined`.
- **The webcontainer flag has to be set before any module evaluates.** Without
  `globalThis.process.versions.webcontainer`, Svelte throws
  `async_local_storage_unavailable` on the first async component
  (`internal/server/render-context.js:39`, `:82-86`). In PR #80 it is the fold's
  banner; in dev it has to be a host global.
- **The dynamic imports nobody awaits must be settled anyway.** Kit and Svelte
  both install AsyncLocalStorage from `import('node:async_hooks').then(...)` at
  module scope. A runner that stops as soon as the entry's own promise settles
  leaves those pending forever, and the first render fails.
- **Per-module lowering breaks resolution in dev.** A module lowered to es2017
  imports `@oxc-project/runtime/helpers/...`, our transform runs after vite's
  import analysis so nothing rewrites it, and under pnpm that package is a
  dependency of vite's core rather than of the app — so an importer inside
  `svelte/` cannot resolve it. The build never sees this. The spike names the
  helper file outright.
- **Go's route table under `vp dev` is the last build's.** `example/server.go`
  reads `skgo.ReadManifest(dist)` from the embedded build in both modes, so a
  route added in dev is invisible until `vp build`. Today that only costs
  routing; under this design the node indices in a render request come from the
  same stale table, and kit's dev numbering is not the manifest's.

## Missions

**1. The engine's modules come from the dev server.** A developer running
`vp dev` gets pages server-rendered by Go, from modules `vp dev` transformed,
with hot updates. Owns: the adapter's environment plugin (dev half) and a new Go
package for the runner. Acceptance: a Gherkin scenario in which a developer edits
a component and the *document bytes* — view-source, before any script runs —
carry the change; and one in which a component that throws only on the server
fails visibly under `vp dev`. Screenshots of both. Kit is the spec for what the
environment contains; PR #80 is the spec for what runs in the engine.

**2. The dev document is kit's dev document.** A page under `vp dev` looks and
hydrates like the built page: kit's dev client entry and app module, per-node
inline styles from vite's module graph with kit's `data-sveltekit` attribute, and
kit's dev boot global. Owns: document assembly and wherever the dev manifest
comes from. Acceptance: the app is styled with no flash, the browser console is
clean of hydration mismatches, and HMR still updates the page — screenshots of
the running app, not of a fixture.

**3. Dev's routing comes from dev.** The routes, node numbering, remote ids and
server-load modules Go uses under `vp dev` describe what vite is serving, not the
last build. Owns: `skgo.Manifest` and its dev source. Acceptance: add a route with
a Go load while both servers are running; it answers without a rebuild.

**4. `ssr = !dev` leaves every app.** The line goes from the example app's root
layout and from `skgo new`'s scaffold, the `@dev`/`@prod` split disappears from
the Gherkin suite, and one suite passes against both modes. That deletion is the
acceptance; nothing else in this brief matters if it does not happen.
