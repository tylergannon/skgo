# skgo semantic index — entrypoint

Routing tree over the **token cache** at `ephemeral/inspiration/` (gitignored, root
checkout only: `/Users/tyler/src/skgo/ephemeral/inspiration/`). Built 2026-09-05 for one
purpose: shipping skgo as described in `ephemeral/brief/2026-09-05-skgo-vision.md` —
a SvelteKit backend in Go, no SSR, generated throwing TS stubs, Playwright as proof.
Content that only served the junkyard's process (ledgers, adjudications, evidence JSON)
is deliberately not indexed beyond the traps it recorded.

## Token cache layout (what is under `ephemeral/inspiration/`)
| Family | Path | What it is | Trust |
|---|---|---|---|
| kit source | `reference/kit/` | pinned `@sveltejs/kit` 3.0.0-next.25 monorepo, commit `a593272` | authoritative |
| devalue | `reference/devalue/` | pinned devalue 5.9.x (kit's serializer) | authoritative |
| sirv, mrmime | `reference/sirv/`, `reference/mrmime/` | static server + MIME table adapter-node uses | authoritative |
| polytype | `reference/polytype/` | mirror of go-gen-jsonschema/polytype v1.0.0-rc.5 + hosted llms.txt | authoritative for rc.5; hosted docs are ahead |
| junkyard | `junkyard/` | the earlier attempt: Go generator, guestbook app + Playwright suite, oracle lab, design docs | evidence; verify against kit source |

## Route order (start here)
1. **Task-first**: `TAXONOMY.md` — "I need to implement X" → the leaves to open, in order.
2. **Cross-cutting**: `themes.md` — facts that span families (ids/hashing, devalue, CSR-only limits, kit-3 breaking changes, traps).
3. **Recipes**: `recipes.md` — copy-paste starting points with `path:line`.
4. **Leaves**: `sources/<family>/<topic>.md` — dense per-source notes with citations.

## Leaf directory
- `sources/kit/remote-server/` — server side of `/_app/remote/*`: ids-and-build, request-handling, serialization, live-and-stream, form-and-prerender.
- `sources/kit/remote-client/` — what the browser sends/expects: client-requests, client-expectations, live-and-batch, client-transform, stub-shape.
- `sources/kit/server-runtime/` — `__data.json`, routing, actions, csr-shell, request-pipeline.
- `sources/kit/client-router/` — CSR boot, data fetching, navigation/manifest, transport hook.
- `sources/kit/build-adapt/` — adapter API, build output, static serving, SPA/prerender, dev server, kit-3 config.
- `sources/kit/portable-tests/` — upstream spec → Go test port map; remote-functions spec; devalue port; serialization/streams; routing/manifest.
- `sources/kit/test-apps/` — kit's own remote/load/no-ssr scenarios as Playwright cases; harness helpers.
- `sources/kit/docs/` — kit-3 facts (corrective list), remote-functions surface, loads/routing surface, junkyard plan facts.
- `sources/libs/` — devalue, sirv, mrmime implementation semantics; polytype pointer.
- `sources/junkyard/go/` — the old Go generator, runtime, CLI, fixture (reuse verdicts).
- `sources/junkyard/remote-codegen/` — old design plan, runtime interface, type glue, lessons.
- `sources/junkyard/colocation/` — link tree (symlinks + routes go.mod), authored example, harness, traps.
- `sources/junkyard/app/` — guestbook app, e2e suite, toolchain pins, CSR portability of each test.
- `sources/junkyard/oracle/` — observed wire facts (tagged by confidence), lab technique, process lessons.

## Citation convention
`<family path>:L<start>-L<end>` relative to the token cache root, e.g.
`reference/kit/packages/kit/src/utils/hash.js:L5-L23`,
`junkyard/go/internal/remotegen/scan.go:L265-L337`. Golden values (hashes, payloads) quoted
inline were computed by running the pinned JS.

## Known debt
- Hosted polytype docs describe rc.6 (`Declare` API) which is not released; leaves say rc.5.
- Kit `documentation/docs/98-reference/*` pages are generator stubs in this checkout; types
  were taken from `runtime/app/server/public.d.ts` instead.
- Vite HMR websocket path facts come from Vite knowledge, not pinned kit source; verify
  against the pinned vite-plus when building the proxy.
- Junkyard oracle observations cover scalar query/command/live only; forms, batch,
  prerender, `__data.json` are source-derived only until skgo's own Playwright runs.
- Linux behavior of `go/packages` through directory symlinks is unverified (macOS only).

## Housekeeping
State and metrics: `.semantic-index/state.json`; evals: `.semantic-index/evals.jsonl`;
benchmark results: `.semantic-index/benchmarks/`. Refresh with the `df-semantic-index`
skill (`.agents/skills/df-semantic-index/`): from = `ephemeral/inspiration/`, to = this dir.
