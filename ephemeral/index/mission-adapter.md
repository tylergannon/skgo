# Mission: the adapter is built from kit's own build

Capability when done: the goja bundle is an output of kit's Vite build (a
fourth environment), folded once into a single script by a second `rolldown()`
call from `adapt()`; the adapter reimplements none of kit's compile, aliases,
defines or remote epilogue; runtime JavaScript is real files; pure data-shaping
polyfills are bound from Go. Every route renders as before, in both modes.

## Read these, in this order (about ten minutes)

1. `ephemeral/brief/2026-09-07-adapter-research-continuation.md` (on main):
   the decision record, nine decisions, the traps, what is not done.
2. `/Users/tyler/src/skgo/ephemeral/inspiration/research/SYNTHESIS.md`: verdict
   per responsibility; round-2 section at the end.
3. The spike that is the design:
   `/Users/tyler/src/skgo/ephemeral/inspiration/research/spikes/environments-prior-art/`
   (`README.md`, `app/`, `twopass.mjs` is the two-pass build, `twopass-bundle.js`
   its 233 KB output, `gojacheck/`). Polyfill spikes:
   `spikes/engine-polyfills/spike1`, `spike2`. Per-agent reports in
   `research/*.md` if a decision needs its evidence (`build-pipeline.md`,
   `remote-and-render.md`, `environments-prior-art.md`, `kit-history.md`).

## The adapter today (`internal/adapter/skgo-adapter.js`, 1,734 lines)

| Lines | What | Fate |
|---|---|---|
| 14 | `import * as esbuild` | delete |
| 43-169 | `adapt()`: reads generated files, checks endpoints/remotes/loads, writes manifest and bundle | keep shape; build call changes |
| 170-297 | `readGenerated`, `checkEndpoints`, `readPrerendered` | keep |
| 298-346 | `readKitManifest`: `builder.generateManifest` (skgo's node numbering is kit's manifest numbering) | keep |
| 347-533 | `readNodes`, `option`, `componentSource`, `describeSSR` | keep or derive from build output |
| 534-661 | `checkRemoteHashes`, `checkRemoteIds`, `checkServerLoads` | keep |
| 687-988 | `SSR_TARGET`, `SSR_POLYFILL` (String.raw, 290 lines: URL, TextEncoder, base64, Headers, Blob, File...) | URL/URLSearchParams/TextEncoder/Decoder/btoa/atob → Go (`goja_nodejs/url`); Headers/Blob/File stay as a small real file (kit uses `instanceof`); into the fold's banner |
| 989-1083 | `SSR_APP_SERVER`: the `$app/server` substitute (`host()`, `query`, `command`, `form`, re-exports) | becomes a virtual module served only to the goja environment; `export * from` kit's real module with overrides; keep the live-batch mission's additions (see Collisions) |
| 1084-1391 | `SSR_ENTRY`: render entry (`handle_error`, `make_state`, `make_event`, `node_data`, `build_props`, `seed_form`) | a real file; borrow kit's `create_request_state` and `handle_error_and_jsonify` |
| 1392-1593 | `buildServerBundle`: esbuild plugin, alias table, Svelte compile, TS strip, defines | delete; replaced by `builder.build(environments.goja)` + one `rolldown()` fold |
| 1594-1734 | `universalHooks`, `resolveEntry`, `ssrDefines`, `globalName`, `nodeTable`, `stripTypeScript`, `djb2` | `ssrDefines`/`stripTypeScript` delete; `nodeTable`, `globalName` keep |

Go side that consumes the bundle: `internal/ssr/ssr.go` `New`:200 loads the
script, `newRuntime`:243 evaluates it per pooled runtime, `Render`:307 calls
the entry; `globals.go` `installGlobals`:36 is where Go-bound polyfills go.
`internal/gen/adapter.go` copies the adapter to `example/web/skgo-adapter.js`
with a fingerprint (`skgo generate` must still match). `cmd/skgo/main.go`
`skgo new` scaffolds an app using the adapter; a fresh app must still build
and render (#67 says a fresh project currently gets `ssr = false`; not yours
to fix, but do not make it worse).

## Kit anchors (`exports/vite/index.js`)

- Environments declared at 385 (ssr sourcemap) and 957; builds at 1158
  (`builder.build(builder.environments.ssr)`), 1261 (client), 1578
  (serviceWorker). The adapter's `vite.plugins` hook is how the goja
  environment is declared; a `config` hook at `order: 'post'`.
- Remote plugin emits one entry chunk per `.remote` module into every server
  environment (656-693, gated by `environment.name !== 'serviceWorker'` at 610
  and again at 1106-1144). That makes the goja environment code-splitting, so
  it must emit esm and be folded by the second rolldown pass with
  `codeSplitting: false`. Do not patch kit; do not file upstream (note it in
  the worklog).
- Kit's adapters locate runtime files from `import.meta.url` (`files/`
  directory): `example/web/node_modules/@sveltejs/adapter-*/index.js` if
  installed, else the pattern is in the brief, decision 7.
- oxc lowers per ES target only; `#field` below es2022 becomes WeakMaps
  (2-7x slower). Lower only the nine modules with `for await`/async
  generators via a `transform` hook (`transformSync(target:'es2017')`).

## Acceptance

Whole suite green in both modes; rendered markup identical to main's adapter
on every route, not just `/` (node numbering diverges after the first
prerendered page); `skgo new` app builds and renders; `skgo generate` writes
a matching fingerprint; esbuild gone from `internal/adapter/package.json`.

## Collisions

The live-batch mission is concurrently adding ~68 lines to `SSR_APP_SERVER`
(live iterator and batch host protocol) and ~130 lines to
`internal/ssr/{ssr.go,globals.go}` (macrotask drain, per-render queue reset).
See its delta with
`git -C /Users/tyler/src/skgo/.claude/worktrees/mission-2-live-batch diff main -- internal/adapter/skgo-adapter.js internal/ssr/`
before you design the `$app/server` virtual module, and port it. Rebase onto
main when it lands (watch `origin/main`); until then keep changes to
`internal/ssr/ssr.go` minimal. The showcase mission does not touch your files.
