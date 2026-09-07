# skgo index

Where things are, so work starts in the first minute instead of the tenth.
Written 2026-09-07 at main `bcf8b5d`. Absolute paths; line numbers are
anchors, not gospel (functions move; `grep -n "^func Name"` finds them).
The mission dossiers beside this file say what each mission touches.

- `mission-live-batch.md` — #61, live and batch queries during render
- `mission-showcase.md` — #63 and the example front page as an index of capability
- `mission-adapter.md` — the adapter rebuilt on kit's own build

## Repository shape

Root module `skgo` (package `skgo`) is the request path. Everything a browser
asks for is answered from these files:

| File | Role | Anchors |
|---|---|---|
| `document.go` | page document: render in goja, assemble, stream | `NewSSR`:69, `serve`:145, `renderPlan`:543, `answer`:792 (host callback answering remote calls made during render), `deliver`:491, `stream`:896 (document chunks `__sveltekit_<hash>.resolve`), `chunkScript`:936, `deferReplacer`:985, `staticErrorPage`:677 (the error.html path), `serveLoadError`:333, `respondWithError`:382, `console`:470 (goja console → Go log), `report`:479 |
| `document_assemble.go`, `document_form.go` | head/body assembly, no-JS form seeding | |
| `data.go` | `__data.json` | `ServeHTTP`:75, `writeNodes`:296, `chunkLine`:351 (`{"type":"chunk",...}`), `promiseReducer`:444, `promiseTable.settled`:487 (settlement order), `header`:431 (`text/sveltekit-data`) |
| `deferred.go` | `Deferred[T]`, `Async`:32, `Resolved`:53; allowed at any depth (kit's rule) | |
| `load.go`, `loadevent.go` | server loads and their event | |
| `remote.go` | `/_app/remote/*` dispatch: query, command, form, prerender | |
| `remote_live.go` | `query.live` as SSE | `serveLive`:36, `sendFrame`:149 |
| `remote_form.go`, `refresh.go` | forms; refreshes after commands | |
| `transport.go`, `transport_encode.go` | the universal `transport` hook, mirrored | `reducers`:128, `revivers`:197, `Transported` marker:238 |
| `endpoint.go` | `+server.ts` routes as `net/http` handlers | |
| `static.go` | kit's client output, immutable assets | |
| `handle.go`, `event.go`, `requested.go`, `proxy.go` (dev proxy), `emptyarray.go`, `jsonfields.go` | | |

`internal/ssr` is the goja engine: `ssr.go` `New`:200, `newRuntime`:243,
`Render`:307, pool `acquire`/`release`:397/416; `globals.go`
`installGlobals`:36 defines `Symbol.asyncIterator`, `Promise.withResolvers`
and a `console` that calls back into Go. The pool is mandatory: a re-entrant
render on one runtime hangs.

`internal/gen` is `skgo generate`: `scan.go` (remote kinds incl. `kindLive`),
`emit.go` (throwing stubs), `loads.go`, `links.go`, `types.go`,
`transport.go`, `endpoints.go`, `adapter.go` (copies the adapter into
`example/web/skgo-adapter.js` with a fingerprint). `cmd/skgo/main.go` is
`skgo new` and `skgo generate`.

`internal/adapter/skgo-adapter.js` (1,734 lines) is the SvelteKit adapter;
its anatomy is in `mission-adapter.md`.

`example/` is the example app: `server.go` and `cmd/` (the Go server),
`businesslogic/store.go` (in-memory store with a Snapshot broadcast),
`generated/` (links, bindings), `web/` (the SvelteKit app), `e2e/` (the
Gherkin suite). Go tests beside them: `ssr_test.go`, `stream_test.go`,
`form_noscript_test.go`, `bindings_test.go`, `typedrift_test.go`.

## Example app routes (`example/web/src/routes`)

A Go file beside a `.remote.ts` or `+page.server.ts` is its implementation;
`skgo_remotes_gen.go` and `types.ts` are generated.

| Route | Shows |
|---|---|
| `/` `+page.svelte` | Home: `Greeting`, `getSite()` in a boundary. Currently not an index of anything. |
| `/about`, `/plain` | prerendered / universal-load pages (`+page.ts`) |
| `/spa` | `ssr = false` branch, kit's SPA fallback |
| `/items/[id]` | params |
| `/todos` (+ `[id]`, `gate`, `pair`) | remote queries, commands, forms, refresh, `query.live` (`todos.remote.go`) |
| `/empty` | nil slice → `[]` on the wire |
| `/api`, `/api/todos` | `+server.ts` routes answered by Go |
| `/(marketing)/pricing` | transported type (`types.ts`, `hooks.ts`/`hooks.go`) in a pending boundary |
| `/docs/[...rest]` | rest params |
| `/account` (+ `orders`, `statement`) | layout server load, nested error page |
| `/contact` | form remote function, works without JavaScript |
| `/stream` | deferred load values on both wires, out-of-order settlement |
| `/console` | render-time `console.error` reaching Go's log |
| `/error/{boundary,command,expected,redirect,render,unexpected}` | each error path |

Nav links: `+layout.svelte`:26-35. Root layout is `+layout.ts` (universal);
there is no root `+layout.server.ts` today. Lib: `src/lib/{Greeting,SignIn}.svelte`,
`auth.remote.*`. Kit 3 facts (no svelte.config.js, `#lib`, transport hook):
`/Users/tyler/src/skgo/ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md`.

## The suite (`example/e2e`)

playwright-bdd 9.2. `features/*.feature` + `steps/*.ts`; shared fixtures in
`steps/fixtures.ts` (Documents / Remotes / Data request counters; the
screenshot helper at :185 writes
`ephemeral/screenshots/<feature>/<mode>/<slug>.png`). Screenshots are looked
at, never committed (`ephemeral/screenshots/` is gitignored).

Justfile variables (lines 8-15): `SKGO_PORT` (8080), `ORIGIN`, `SKGO_LOG`
(default `example/e2e/server.log`). Gestures:

```
just ports                  # what is listening already; pick a free SKGO_PORT
just build                  # fresh tree: builds the frontend
SKGO_PORT=8123 just serve   # Go server on the built frontend, log tees to SKGO_LOG
ORIGIN=http://127.0.0.1:8123 just e2e         # prod mode
just dev + ORIGIN=... just e2e dev            # dev mode (vite dev behind the Go proxy)
just test                   # go test, both modules
```

One feature only, while iterating (playwright filters by path substring):

```
cd example/e2e && BASE_URL=$ORIGIN EXPECTED_MODE=prod SKGO_LOG=$LOG mise x -- pnpm test -- live
```

Full suite once per mode at the end. A run is about two minutes per mode;
mutation checks run the touched feature, not the suite.

Kill servers by pid you own. The "listening on" line prints before the bind.

## Kit anchors

`K=/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/src`
(kit@3.0.0-next.25; line numbers match `example/web/node_modules/@sveltejs/kit/src`).
Svelte's source: `example/web/node_modules/svelte/src`.

- Page streaming: `runtime/server/page/render.js`:662 (`stream_text`),
  `runtime/server/page/data_serializer.js` (`get_replacer`, iterators at 19 and
  133; promises deferred at any depth), `utils/streaming.js`:11
  (`create_async_iterator`, settlement order).
- `__data.json`: `runtime/server/data/index.js`:112-125.
- Remote functions, server: `runtime/server/remote-functions.js`:35
  `create_live_query_response` (SSE frames `result`/`redirect`/`error`), :205
  live dispatch, :208 `query_batch`. `runtime/app/server/remote/query.js`:186
  and :203 live during render, :248 `batch`, :515 `create_live_query_resource`
  (awaits the first yielded value during SSR). Also `command.js`, `form.js`,
  `prerender.js`, `requested.js` in that directory.
- Remote functions, browser: `runtime/app/remote/` and
  `runtime/client/remote-functions/`, `runtime/client/sse.js`,
  `runtime/client/ndjson.js`, `runtime/client/stream.js`.
- Vite plugin: `exports/vite/index.js` environments at 385 and 957; remote
  plugin emits an entry chunk per `.remote` module at 656-693 (blocklist
  `environment.name !== 'serviceWorker'`, 610 and 1106); builds at 1158
  (ssr), 1261 (client), 1578 (serviceWorker); `adapter.vite.plugins` hook.
- Manifest: `core/generate_manifest/index.js`:37-58 reindexes nodes,
  dropping prerendered-only ones. skgo's manifest is
  `builder.generateManifest` (adapter :307-310), so skgo's numbering *is*
  kit's manifest numbering; `.svelte-kit/output/server/nodes/N.js` keeps the
  pre-reindex numbers. Any parity check must include a route after the first
  prerendered page.

## Traps, all paid for

- A render that leaves work queued on a runtime poisons the pooled runtime
  for the next render (found in mission 2; the fix drains and resets per
  render in `internal/ssr/ssr.go`).
- The folded bundle evaluates kit's shared chunk before the entry, so the
  polyfill must be in the fold's banner, not a module.
- Expectations derived from the thing under test pass when it is broken.
  Anchor in a fixture, a number in the scenario, rows on the page.
- `gh pr view --json files` stops at 100 files; use
  `gh api repos/<o>/<r>/pulls/<n>/files --paginate`.
- Dev and prod are different servers; the suite runs in both modes and a
  feature can pass in one and fail in the other (`dev.feature` is the
  reference for mode-specific steps).
- goja: no `Symbol.asyncIterator`, `Promise.withResolvers`, `console`
  natively (installed at runtime creation now); no timers by design.
- `ephemeral/inspiration` exists only in the root checkout; refer to it by
  absolute path. Never copy it into a worktree.
