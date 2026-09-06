# Sprint 003: Bindings generation and server loads, CSR

Closes #4, #2, #3 (PR A) and #5 (PR B). Two worktrees, three Opus builders, PR B rebases
onto main after PR A merges. This document de-risks and draws seams; the builders choose
packages, names, and commands. One critique round applied (2026-09-06).

## Pyramid Index

- L0: Go becomes the source of truth for kit's `.remote.ts` stubs, Go commands can refresh
  argument-taking queries, and Go answers `+page.server.ts` loads over `__data.json` — all
  proven against a production build with Vite stopped, and again behind `vp dev`.
- L1:
  - Generator: emit `.remote.ts` (types + throwing stubs) from Go registrations; delete the
    hand-written stub; suite stays green.
  - Refresh: a typed way for a `Command` handler to refresh `getTodo(id)` in the same round
    trip. First Go-constructed payload; #2 (UTF-16 sort) and #3 (`nil`≠`undefined`, map order)
    get fixed against real tests.
  - Loads: Go matches `<path>/__data.json` on the route table it already has, answers kit's
    NDJSON envelope from a `Load` binding; page loads only; `uses.params` honored.
  - Proof: Playwright BDD against the binary; `go test`; both modes.
- L2: `drafts/SPRINT-003-research-bindings.md`, `drafts/SPRINT-003-research-data-json.md`,
  issues #2–#5.

## Facts that de-risk this (verified against pinned kit next.25)

Bindings (`drafts/SPRINT-003-research-bindings.md`):
- Kit needs only: the file exists at `vp build`, is imported by app code, and every runtime
  export is a top-level `query`/`command`/`query.live` wrapper with `'unchecked'`. Bodies are
  discarded on the client. Exported helpers break the build; type-only exports are fine.
- Dev picks up regenerated files (add/unlink → full reload; edit → HMR). No restart.
- `*Remote` erases `In`/`Out` and hides kind — the generator cannot reflect over registrations
  until `remote.go` retains them. That change is in scope.
- polytype rc.9 is latest (`@latest` lies — pin it), but its TS projector is CLI-only, drops
  `*T` nullability, and rejects maps. **Decision: a small reflect-based emitter in skgo that
  mirrors `encoding/json` exactly** (`*T` → `T | null`, `map[string]V` → `Record<string,V>`,
  json tags, `omitempty`/`omitzero` → optional, `[]byte` → string, `any` → `unknown`,
  `time.Time` → `string`). Revisit polytype when unions/enums appear or its projector is
  importable.
- The module path is dual-owned today (`example/remotes.go:15` vs the file). The generator
  must own it, and `kithash.Kit` of the derived path must equal today's `worolc` — any
  normalization drift (`./`, separators) 404s every remote scenario with no build error.
- Server-pushed `q` entries are applied by the client without it having asked
  (`shared.svelte.js:139-150`) — but a key that does not match the client's own
  `stringify_query_arg` bytes is silently dropped. `r: true` is read only by `form`, never by
  `command`; do not design around it.

Loads (`drafts/SPRINT-003-research-data-json.md`):
- Cold SPA boot fetches `<path>/__data.json` — so #5 is a real CSR feature. Query params
  `x-sveltekit-invalidated` (bitstring over `[...layouts, leaf]`) and
  `x-sveltekit-trailing-slash`; no request headers.
- The leaf is always the **last** position (`parse.js:23-31`). With page loads only, Go sizes
  `nodes` from the bitstring length and emits `[null × (n-1), {"type":"data",…}]`. No server
  manifest parsing, no node indices — `builder.routes` (id, pattern, params) is enough, and
  `static.go` already compiles those patterns.
- Response is NDJSON with a trailing `\n`: `{"type":"data","nodes":[…]}`. Node:
  `{"type":"data","data":<devalue flat>,"uses":{…}}`, same devalue reducers as remotes.
  Redirect is HTTP 200 `{"type":"redirect"}`. Top-level error is `App.Error` JSON at the
  status with `application/json`. Omit `x-sveltekit-version` or echo kit's.
- **`uses` is load-bearing.** If omitted, `has_changed` (`client.js:1332-1361`) is false for
  everything and a param change never refetches — stale data, no error. `invalidateAll`
  bypasses `uses`, so it proves nothing about them.
- The client only asks for nodes kit compiled as having a server load: a throwing
  `+page.server.ts` exporting `load` must exist at build. In prod no server JS ships, so
  "the stub throws" is only observable under `vp dev`.
- **Kit's `[^]` is JS-only regex; RE2 rejects it.** `static.go:104-111` already compiles
  patterns without the rewrite — a `[...rest]` route would fail at startup today.
- `vp dev` answers `__data.json` itself → Go intercepts before proxying, but only for routes
  with a registered Go `Load`; everything else passes through (dev → kit, prod → 404 empty
  body, matching `data/index.js:29-32`).

## Seams

**A — typed remotes + generator + refresh wiring.** Owns `remote.go`, `remote_live.go`, the
generator package and its `go generate` gesture (a separate `package main`, not a second
one under `example/cmd/`), `example/remotes.go`, the regenerated `todos.remote.ts`,
`example/web/src/routes/todos/**`, `example/web/src/lib/*.svelte`, and new scenarios in
`remote.feature`. Exposes: registrations that retain kind and `In`/`Out` reflect types; a
typed `Refresh` usable from a `Command` handler that composes `<hash>/<name>/<payload>` and
fills the `q` node. Pins the expected key bytes (captured from the browser) in its own test
so it does not depend on B's timing. Must not edit `internal/devalue`, `internal/remotearg`,
`static.go`, `example/cmd/main.go`, or e2e fixtures.

**B — codec correctness (#2, #3).** Owns `internal/remotearg` and `internal/devalue`.
Guarantees `StringifyQueryArg` is byte-identical to kit's client for a corpus including
astral vs private-use strings (UTF-16 code-unit ordering), `devalue.Undefined` vs `nil`, and
that command-arg stringify refuses `map[string]any` rather than silently sorting. Pure
library + tests; no example changes. Small — lands first.

**C — loads.** Owns `static.go` (one route matcher, extended, with the `[^]` rewrite), the
`__data.json` dispatcher, the `Load` binding API, `example/cmd/main.go` wiring in both modes,
`example/web/skgo-adapter.js` if the route table needs anything more, a **new route** (not
`/todos/[id]`, which existing scenarios cover) with its `+page.server.ts` throwing stub and
`.svelte` files, `loads.feature`, and a `__data.json` counter in fixtures. Registers loads by
kit route id with a retained `Out` type. Must not edit `remote.go` or the codec packages.

**Deferred to Sprint 004:** the generator emitting `+page.server.ts` stubs from `Load`
registrations. First thing dropped; not in either PR.

## Functional Definition of Done

PR A (closes #4, #2, #3):
1. The hand-written `todos.remote.ts` is gone; the generator reproduces it from Go; `vp
   build` + the existing Playwright suite pass unchanged; a Go test asserts the derived
   module path still hashes to `worolc`.
2. A Go command invoked **without** `.updates()` on the client mutates a todo, and the open
   `/todos/[id]` page shows the new text from that one round trip — the `remotes` counter
   proves no second request, so the `q` node demonstrably originated in Go.
3. #2 and #3 have failing-then-passing tests; `go vet`, `go test` green; dev mode still
   answers remote calls.

PR B (closes #5):
4. Production binary, Vite stopped: a deep link to the new route renders data that only Go
   has, on cold boot.
5. Client-side navigation between two params of that route refetches and shows the other
   param's data (`uses.params` honored); `invalidateAll()` re-fetches and reflects a
   server-side change.
6. Redirect and error envelopes unit-tested against the captured shapes.
7. `vp dev` behind the Go binary: the same route works with the JS stub throwing, and a route
   without a Go `Load` still reaches kit's dev server.
8. Adapter refuses to emit a Go-loaded route that is also prerendered (cheap assertion).

## Shrink order if the session blows out

Sprint-004 stub emission (already out) → exact TS type projection in A (emit `unknown`
signatures; kit does not need the types) → the astral/private-use corpus in #2 (keep #3, it
is cheap and it is what the code actually gets wrong). Keep DoD 6: pure unit tests.

## Advice and traps

- Track `uses` by observation: a `Load` that reads a param or the URL through the event gets
  `uses.params`/`uses.url`; a `Depends(...)` covers `invalidate('…')`. Coarse `url: 1`
  everywhere works but refetches on every search-param change.
- Layout server loads, `{"type":"skip"}`, streaming promises, `form`, `query.batch`,
  `prerender`, SSR: out.
- The generator must never emit an exported helper; keep `unimplemented` module-private.
- `example/web/node_modules` must be installed and `vp build` run before `go vet ./example/...`
  passes (`//go:embed all:build`).
- Three `vp` binaries exist; only the project-local one works (`example/mise.toml` `_.path`).
- Token cache: `ephemeral/inspiration/` is a directory of symlinks, gitignored. In a worktree,
  symlink it to the root's copy and **never `git add` it** (`.gitignore` now covers the link).
