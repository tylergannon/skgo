# Sprint 001 Intent: Bare Go server fronting a SvelteKit app (dev proxy + embedded prod build)

## Seed

discuss with me what you think we can produce in one sprint.  it MUST lead to an actual working server that's actually rendering SvelteKit routes, without any remote functions, proxied through the Go server, with Air and `viteplus` both configured and working together.

help me choose a bite that we can knock out, and let's make a sprint plan.

## Context

- First sprint. The repo contains only AGENTS.md, the verbatim brief
  (`ephemeral/brief/2026-09-05-skgo-vision.md`), the build plan
  (`ephemeral/plans/2026-09-05-skgo-build-plan.md`), and the semantic index. No Go or
  JS code exists yet. AGENTS.md still describes the old design; the brief and plan win.
- Decisions taken with Tyler on 2026-09-05 for this sprint:
  - **Adapter: `@sveltejs/adapter-node`** (kit's build must keep producing the client
    side of remote functions for later sprints; it does regardless of adapter, but
    adapter-node is the chosen build vehicle). The Node server output is dead weight:
    the production Go binary embeds only `build/client` + `build/prerendered`.
  - **Production requirement:** `go build ./example/cmd` produces a binary that, with
    Vite NOT running, serves the compiled SvelteKit app from an `embed.FS` to a real
    browser.
  - **Dev requirement:** Go proxies to `vp dev` (URL from a CLI flag), Air rebuilds the
    Go binary, and **overmind** (Procfile) runs the processes together.
  - **Proof:** `playwright-bdd` in `example/e2e/` (own package) with simple scenarios
    proving real Svelte components render through Go, run against `BASE_URL` in both
    dev (Go+overmind) and prod (bare binary) modes.
  - **No remote functions, no server loads, no codegen** this sprint.
  - Sprint size: one focused agent session.
- The HTML document under `ssr = false`: kit does not write an HTML file into
  adapter-node output; its Node server renders the client-boot document per request.
  Plan: `export const ssr = false; export const prerender = true;` in the root
  `+layout.ts`, `paths: { relative: false }` in the kit config, so kit prerenders `/`
  as a route-independent boot document with absolute asset paths; Go serves that file
  for every non-asset path. Dynamic route `/items/[id]` sets `prerender = false`.
  Fallback if that fails: Go synthesizes the document (see
  `ephemeral/semantic-index/sources/kit/server-runtime/csr-shell.md`).

## Pyramid Index

- L0: Stand up the repo shape and a Go server that fronts a kit 3 app in dev (proxy to `vp dev`, Air, overmind) and in prod (embedded build), proven by playwright-bdd scenarios.
- L1:
  - Repo shape per brief: root `go.mod github.com/tylergannon/skgo`, `example/` module, `example/web` kit app, `example/e2e` Playwright package.
  - `skgo` library: reverse proxy with WebSocket upgrade (HMR) + embedded static server with catch-all document; both as `http.Handler`s; CLI flags in `example/cmd`.
  - Toolchain pins from the junkyard leaf: kit 3.0.0-next.25, adapter-node 6.0.0-next.x, svelte 5.x, vite-plus 0.3.0 via mise, TypeScript 6.0.3 (not 7), pnpm via devEngines, `#lib` imports map, `$app/tsconfig`, `paths.origin`.
  - Proof: playwright-bdd features (home renders a Svelte component, client nav without document reload, deep link to `/items/[id]`, unknown route shows error page) against dev and prod `BASE_URL`; Go unit tests for proxy (incl. websocket) and static handler.
  - Risks: kit-3 config traps (origin, TS 7, `svelte.config.js`), `.svelte-kit`/embed path layout, HMR websocket through the proxy, prerendered document with `paths.relative`.
- L2:
  - Repo shape → brief §"Start with the right shape"; plan §1.
  - Kit facts → `ephemeral/semantic-index/sources/kit/docs/kit3-facts.md`, `sources/junkyard/app/toolchain.md`.
  - Dev proxy → `sources/kit/build-adapt/dev-server.md`. Static serving → `sources/kit/build-adapt/static-serving.md`, `build-output.md`. Document → `sources/kit/server-runtime/csr-shell.md`, `sources/kit/client-router/csr-boot.md`.
  - Playwright idioms → `sources/junkyard/app/e2e-suite.md`, `sources/kit/test-apps/harness-helpers.md`.

## Semantic Index

- **Available** — Token cache: `/Users/tyler/src/skgo/ephemeral/inspiration/`. Index
  entrypoint: `/Users/tyler/src/skgo/ephemeral/semantic-index/README.md` (config pointer:
  `ephemeral/SEMANTIC-INDEX.md`; there is deliberately no `docs/` directory in this repo).
  Access: read the entrypoint, then `TAXONOMY.md` route **A. Stand up the bare Go server**,
  open the leaves it lists; use `rg` against the token cache for targeted searches.
  Prior art retrieved: junkyard toolchain pins and Playwright launch line under mise;
  adapter-node's static-serving rules; kit's dev middleware chain (Go must intercept
  nothing this sprint — everything proxies); the CSR boot contract (document, `_app/immutable`,
  `version.json`); kit-3 breaking facts.

Planning agents drafting this sprint must read the index entrypoint, follow route A, and
open the cited leaves before proposing implementation approaches. Do not scan the token
cache; sniff via the index. Do not read `ephemeral/inspiration/junkyard/ephemeral/**`
directly — its process artifacts are indexed only for traps.

## Chapter Context

No chapter link selected (no `docs/chapters/` in this repo; chapters are not used).

## Recent Sprint Context

First sprint.

## Relevant Codebase Areas

Nothing exists yet. Target layout (from the brief, refined in the plan):

```
skgo/                    go.mod github.com/tylergannon/skgo
  proxy.go|static.go     library handlers (or internal/proxy, internal/static)
example/                 go.mod github.com/tylergannon/skgo/example
  cmd/main.go            flags: --listen, --proxy URL (dev) | embedded mode (default)
  web/                   vite root: package.json, vite.config.ts, src/, mise.toml
  e2e/                   playwright-bdd package: features/, steps/, playwright.config.ts
  Procfile               overmind: vp dev + air
  .air.toml
```
Where the built kit output lands for `go:embed` (an `embed.FS` cannot reach outside its
package directory) is a design point the drafts must settle: e.g. adapter-node `out` set to
`example/web/build` and an embedding package `example/web/dist.go`, or copying into
`example/cmd/`.

## Constraints

- Must follow AGENTS.md rules that still apply: Go library, `go test`, no JS except where
  kit requires it (`vite.config.ts`, app code, the Playwright suite); no ledgers/proof
  machinery; worklog for actionable intelligence only.
- No SSR anywhere; no Node process in production; Go never spawns Node.
- Bleeding-edge pins: kit `next`, vite-plus 0.3.0, Svelte 5; TypeScript must stay 6.x.
- The kit app's origin is fixed at build time (`paths.origin`); Go must listen there.
- Sprint docs live in `ephemeral/sprints/` (ledger: `ephemeral/sprints/ledger.yaml`).
- Process orchestration in dev is overmind + Procfile; Go only proxies to a URL.

## Success Criteria

1. `overmind start` in `example/` runs `vp dev` and Air; a browser at the Go port shows the
   app, client navigation works, editing a `.svelte` file hot-updates through the proxy,
   editing Go restarts the server.
2. `vp build` then `go build ./example/cmd` then running the binary with Vite stopped
   serves the same app from the embedded FS to a real browser.
3. `playwright-bdd` scenarios pass against both modes via `BASE_URL`.
4. `go test ./...` passes at the root and in `example/`; `go vet` clean.

## Open Questions

- Exact embed layout (see above) and whether a stale/missing build should fail `go build`
  (e.g. a `//go:generate` that runs `vp build`, or a checked-in placeholder).
- Whether the prerendered `/` document with `paths.relative: false` is byte-identical in
  boot behavior to kit's `generateFallback` output for deep links (verify with the
  deep-link scenario).
- Minimal static-serving correctness to include now: `Cache-Control: immutable` for
  `/_app/immutable/*` and precompressed `.br/.gz` are cheap; full sirv parity is later.
- WebSocket proxying: `httputil.ReverseProxy` handles HTTP/1.1 upgrades natively in Go ≥1.12
  — confirm with a test against `vp dev`'s HMR endpoint.
