# Sprint 001 — Bare Go server fronting a SvelteKit app (draft, Claude)

Source intent: `ephemeral/sprints/drafts/SPRINT-001-INTENT.md`. Brief:
`ephemeral/brief/2026-09-05-skgo-vision.md`. Build plan:
`ephemeral/plans/2026-09-05-skgo-build-plan.md`. Semantic index entrypoint:
`ephemeral/semantic-index/README.md`, routed via `TAXONOMY.md` route **A** (leaves
opened and cited throughout — see each section's citations).

Note on AGENTS.md: it still describes the old adapter+sidecar+SSR design. Per the
intent's Context section, that description is superseded by the brief and this
plan for everything below; AGENTS.md itself needs a follow-up rewrite (tracked as
an open question, not in this sprint's scope).

---

## Pyramid Index

- **L0**: Stand up a Go server that fronts a SvelteKit 3 (kit-3) CSR-only app —
  proxying to `vp dev` in development (with Air + overmind) and serving an
  embedded production build — proven by `playwright-bdd` scenarios run against
  both modes via a Go-driven test harness.

- **L1**:
  - Repo shape: root module `github.com/tylergannon/skgo` (packages `proxy`,
    `static`); `example/` module `github.com/tylergannon/skgo/example`
    (`cmd/main.go`, `web/` kit app, `e2e/` playwright-bdd package).
  - Go library: `proxy.New(target *url.URL) http.Handler` (reverse proxy +
    websocket upgrade for HMR) and `static.NewHandler(fs.FS, shellHTML []byte,
    Manifest) http.Handler` (adapter-node-equivalent static serving + CSR
    shell fallback with route-aware 200/404 decision), both `net/http.Handler`s
    wired by `example/cmd/main.go` behind a `--dev-proxy` flag.
  - Toolchain pins (junkyard-validated): kit `3.0.0-next.25`, svelte `5.56.10`,
    vite `8.2.2`, `@sveltejs/vite-plugin-svelte` `7.3.0`, `@sveltejs/adapter-node`
    `6.0.0-next.10` (build vehicle only, discarded at runtime), TypeScript
    `6.0.3`, `@types/node` `26.4.1`, mise-provisioned Node `24.16.0` +
    `npm:vite-plus@0.3.0`; new pins for this sprint: `air-verse/air` `v1.67.4`,
    `DarthSim/overmind` `v2.5.1` (both via mise), `playwright-bdd` `9.2.0`,
    `@playwright/test` `1.62.1`.
  - Proof: a Go test (`example/e2e_test.go`, build-tagged `e2e`) that builds the
    kit app (`ORIGIN=... vp build`), builds the Go binary, starts it, and shells
    out to `playwright-bdd`'s generator + Playwright CLI against `BASE_URL`; run
    once against the embedded prod binary and once against `overmind start`'s
    dev stack. Go unit tests for `proxy` (incl. websocket upgrade) and `static`
    (adapter-node/sirv parity table tests) run under plain `go test ./...`.
  - Risks: `//go:embed` silently drops kit's `_app` directory unless the `all:`
    prefix is used; `embed.FS` reports zero `ModTime`, breaking sirv's
    mtime-based ETag formula; the single prerendered `/` shell's
    modulepreload list is specific to `/`, not to `/items/[id]`; HMR
    websocket-through-proxy behavior is Vite knowledge, not pinned kit source;
    `playwright-bdd`'s exact config API is outside the semantic index and must
    be verified hands-on.

- **L2** (citations, `<family path>` relative to `ephemeral/inspiration/`):
  - Static serving → `sources/kit/build-adapt/static-serving.md`,
    `sources/kit/build-adapt/build-output.md`, `sources/libs/sirv.md`,
    `sources/libs/mrmime.md`.
  - CSR boot/shell → `sources/kit/server-runtime/csr-shell.md`,
    `sources/kit/client-router/csr-boot.md`,
    `sources/kit/build-adapt/spa-and-prerender.md`.
  - Dev proxy → `sources/kit/build-adapt/dev-server.md`.
  - Request pipeline (base/appDir 404s, CSRF shape for later) →
    `sources/kit/server-runtime/request-pipeline.md`.
  - Kit-3 config/facts → `sources/kit/build-adapt/config-kit3.md`,
    `sources/kit/docs/kit3-facts.md`.
  - Adapter surface (why we don't write a custom adapter yet) →
    `sources/kit/build-adapt/adapter-api.md`.
  - Toolchain pins and Playwright launch line → `sources/junkyard/app/toolchain.md`.
  - Acceptance-suite prior art → `sources/junkyard/app/e2e-suite.md`,
    `sources/kit/test-apps/harness-helpers.md`.
  - Cross-cutting → `themes.md` T3 (CSR-only contract), T5 (kit-3 breaks),
    T8 (Playwright is protocol-agnostic proof).

---

## Overview

This sprint builds the **first runnable slice** of skgo: a Go binary that serves
a plain kit-3 SvelteKit app with **no remote functions, no server loads, no code
generation**. In development, Go is a thin reverse proxy in front of `vp dev`
(Vite+'s dev server), with Air rebuilding/restarting the Go binary on `.go`
changes and `overmind` running both processes from one `Procfile`. In
production, `vp build` (via `@sveltejs/adapter-node`, used only as a build
vehicle) produces `build/client` and `build/prerendered`; the Go binary embeds
both trees with `//go:embed` and serves them with no Node process at all. A
single prerendered `/` page (root layout: `ssr = false`, `prerender = true`,
`paths.relative = false`) becomes the universal CSR boot shell Go serves for
every page route.

Proof is `playwright-bdd` (Gherkin features + Playwright) run against a
`BASE_URL`, executed in both modes by a Go test that shells out to `vp
build`/`vp dev`/the Go binary/the Playwright CLI — matching the project rule to
test with `go test` while keeping the actual browser-level proof in Playwright,
per Tyler's standing instruction that Playwright, not code review or a proof
DSL, is what proves the wire protocol works.

This sprint deliberately excludes: remote functions, `__data.json`/server
loads, the link-tree/colocation machinery (not needed — no route directories
contain Go files this sprint), devalue, CSRF enforcement (no mutating routes
exist yet), and any custom skgo adapter (`adapter-node`'s stock output is
sufficient). All of that is Phase B/C/D per the build plan.

---

## Use Cases

1. **Developer runs `overmind start` in `example/`.** `vp dev` boots on
   `127.0.0.1:5173`; Air builds and starts the Go binary listening on
   `127.0.0.1:4173` with `--dev-proxy http://127.0.0.1:5173`. A browser at
   `http://127.0.0.1:4173/` shows the app; client-side navigation to
   `/items/42` does not reload the document; editing `src/routes/+page.svelte`
   hot-updates in the browser without a full reload; editing a `.go` file
   restarts the Go process without touching Vite.

2. **CI/operator builds for production.** `ORIGIN=http://127.0.0.1:4173 mise x
   -- vp build` (from `example/web/`) produces `build/client` and
   `build/prerendered`; `go build ./example/cmd` produces a single binary;
   running that binary with `--listen :4173` (Vite stopped, no Node process
   anywhere) serves the same app to a real browser from the embedded
   filesystem.

3. **Deep link.** A browser requests `/items/42` directly (no prior
   navigation). Go recognizes it as a known page-route pattern, serves the CSR
   shell with `200`, and kit's client boots, matches the route client-side,
   and renders `Item #42` from the URL param alone (no data fetch — there is
   no server load this sprint).

4. **Unknown route.** A browser requests `/does-not-exist`. Go does not match
   any known page-route pattern, so it serves the shell with a real HTTP `404`
   status; kit's client router still boots and renders its own
   `+error.svelte` with status 404 inside the page.

5. **Proof run.** `go test -tags e2e ./example/...` builds the kit app, builds
   the Go binary, starts it on a free port, runs `playwright-bdd`'s generator
   and the Playwright CLI against it, and fails the Go test if any scenario
   fails or is unexpectedly skipped. The same test (parameterized) also drives
   the dev stack (`vp dev` + the Go binary in dev-proxy mode) through the
   identical scenarios.

---

## Architecture

### Repository shape

```
skgo/                                    go.mod: github.com/tylergannon/skgo
  go.mod
  mise.toml                              go=1.27.1, air=1.67.4, overmind=2.5.1
  proxy/
    proxy.go
    proxy_test.go
  static/
    static.go
    mimetable.go
    mimetable_gen.go                     # generated, committed (see Dependencies)
    gen/mimetable_gen.go.gen.go          # //go:build ignore generator
    static_test.go
    testdata/                            # ported subset of sirv's tests/public/
  AGENTS.md -> CLAUDE.md (unchanged)

example/                                 go.mod: github.com/tylergannon/skgo/example
  go.mod                                 replace github.com/tylergannon/skgo => ../
  cmd/
    main.go
  web/
    package.json
    vite.config.ts
    tsconfig.json
    mise.toml                            node=24.16.0, npm:vite-plus=0.3.0
    src/
      app.html
      routes/
        +layout.ts                       ssr=false; prerender=true
        +layout.svelte
        +page.svelte                     home page
        items/
          [id]/
            +page.ts                     prerender=false
            +page.svelte                 renders `Item #{params.id}`
      static/
        favicon.svg
  e2e/
    package.json                         playwright-bdd, @playwright/test
    playwright.config.ts
    features/
      home.feature
      navigation.feature
      deep-link.feature
      unknown-route.feature
    steps/
      common.ts
  e2e_test.go                            //go:build e2e — Go-driven proof harness
  Procfile                               overmind: vite + go(air)
  .air.toml
```

No `src/routes/go.mod` boundary and no colocation link tree this sprint: no
route directory contains a Go file yet (no `page.server.go`/`data.remote.go`),
so the machinery in `sources/junkyard/colocation/link-tree.md` is out of scope
until Phase B/C.

### Go packages

```go
// package proxy — github.com/tylergannon/skgo/proxy
func New(target *url.URL) http.Handler
```
A thin wrapper over `httputil.ReverseProxy` with a `Director` that forwards the
browser's `Host` header unchanged (dev-server.md's recommendation — kit derives
its per-request origin from `Host`/`:authority`, so forwarding it keeps
`event.url.origin` equal to what the user typed) and sets
`X-Forwarded-Host`/`X-Forwarded-Proto`. Go's `httputil.ReverseProxy` forwards
`Upgrade`/`Connection` and proxies WebSocket upgrades natively (confirmed
behavior since Go 1.12; must still be verified against `vp dev`'s actual HMR
endpoint per `sources/kit/build-adapt/dev-server.md`'s open item — no kit source
governs Vite's HMR wire format). In dev mode **every** request is proxied
verbatim; there is nothing to intercept this sprint (no `__data.json`, no
`/_app/remote/*`, no Go actions/endpoints exist yet), which is a deliberate
simplification of the general dev-router recipe in `dev-server.md`.

```go
// package static — github.com/tylergannon/skgo/static
type RouteMatcher func(path string) (isPageRoute bool)

type Manifest struct {
    AppDir string        // "_app" — from config-kit3.md's default
    Routes RouteMatcher  // decides 200 vs 404 for the shell fallback
}

func NewHandler(client fs.FS, shell []byte, m Manifest) http.Handler
```
Lookup order per request, mirroring adapter-node's own `serve(client) →
serve_prerendered() → fallback` chain (`static-serving.md` §"adapter-node
request pipeline"), collapsed for this sprint's single-shell-document design:

1. Exact-file lookup under `client` (which already contains everything
   `writeClient` wrote: `static/`'s copied contents, `_app/version.json`,
   `_app/manifest.js`, `_app/immutable/**`). No sirv extension-candidate
   resolution is needed this sprint (kit's client tree never needs `/foo` →
   `foo.html` resolution — that's sirv's `single`/fallback behavior for a
   different use case). A hit under `/<AppDir>/immutable/` gets
   `Cache-Control: public,max-age=31536000,immutable`; every other hit gets a
   weak ETag and `Cache-Control: no-cache`.
2. A miss under `/<AppDir>/` (any other `_app/*` path) → `404` with
   `Cache-Control: public, max-age=0, must-revalidate` and an empty body,
   matching kit's own `/_app/*` 404 rule (`request-pipeline.md` step 12) —
   never falls through to the shell.
3. A miss that looks like a static asset (has a file extension in the last
   path segment, sirv's `ignores` heuristic from `sirv.md`) → plain `404`.
4. Otherwise: run `m.Routes(path)`. Match → serve `shell` with `200`. No match
   → serve the same `shell` bytes with `404`. Both cases:
   `Content-Type: text/html; charset=utf-8`, `x-sveltekit-page: true`.

`Routes` is a small hand-written matcher for this sprint's two page routes
(`/` and `/items/{id}`), not a manifest parse — there is no codegen or adapter
work this sprint to produce a real route list. ETags are computed once at
process start from a SHA-256 of each embedded file's bytes (`embed.FS` always
reports a zero `ModTime`, so sirv's `W/"<size>-<mtimeMs>"` formula is
meaningless here — a documented, deliberate deviation, not a bug). No
Range/precompression support this sprint (no Playwright scenario in this
sprint's scope exercises either); the full sirv/mrmime port (Range, `.br`/`.gz`
negotiation, prod cache-walk semantics) stays scoped to `sirv.md`/`mrmime.md`
for whichever later sprint needs Playwright's `platform.spec.ts`-equivalent
static-asset scenarios.

The MIME table is generated, not `mime.TypeByExtension` (which reads
OS-specific files at init and disagrees with mrmime on `.js`, `.css`, `.map`,
`.webmanifest`, etc. — `mrmime.md`'s measured diff table). `static/gen/` holds
a `//go:build ignore` generator that parses
`ephemeral/inspiration/reference/mrmime/deno/mod.ts` and emits
`static/mimetable_gen.go` (438 entries, sorted, source commit `c95e4bf` in a
comment) — invoked manually this sprint (`go run ./static/gen
> static/mimetable_gen.go`) since `ephemeral/inspiration/` is gitignored and
not available in CI; the generated file is committed.

### The embedded build

```go
// example/web/dist.go
package web

import "embed"

//go:embed all:build/client all:build/prerendered
var BuildFS embed.FS
```
**Trap, must not be missed**: kit's default `appDir` is `"_app"` — an
underscore-prefixed directory (`config-kit3.md` default table). Go's
`//go:embed` silently excludes any file or directory starting with `_` or `.`
*unless* the pattern carries the `all:` prefix. Without `all:`, the entire
`_app/` tree (every JS/CSS chunk) would be silently missing from the binary
with no build error — only a 404 storm at runtime. `example/cmd/main.go` calls
`fs.Sub(web.BuildFS, "build/client")` to get the root the `static` package
expects, and reads `build/prerendered/index.html` (via `fs.ReadFile`) as the
`shell` argument.

### CSR shell strategy and its known gap

Root layout sets `ssr = false; prerender = true;` and kit config sets
`paths.relative = false` with `paths.origin` fixed to the build's `ORIGIN`
(`config-kit3.md`). Kit's normal build-time prerender crawl (which runs
regardless of adapter — `spa-and-prerender.md`) therefore writes
`prerendered/pages/index.html`: with `ssr = false` this is an *empty shell*
(`rendered.body = ''`, no branch data — `csr-shell.md` §"When the shell is
produced"), not real content, and because `paths.relative` is off its asset
references are absolute — safe to serve at any request depth.
`/items/[id]/+page.ts` sets `prerender = false` (page options must come from a
`.js`/`.ts` file, never the `.svelte` component — `kit3-facts.md`), so the
crawler never visits it and it is rendered purely client-side.

**Documented gap, inherited verbatim from the intent** (`SPRINT-001-INTENT.md`
§"The HTML document under `ssr = false`"): the `/` shell's `<link
rel="modulepreload">` list reflects `/`'s own node imports
(`client.imports ∪ that page node's imports`, `csr-boot.md` §"Head for a CSR
page"), not `/items/[id]`'s. Reusing it for every route is not a *correctness*
bug — the client still lazily imports `items/[id]`'s chunk during the `enter`
navigation (`csr-boot.md` §"What the client does on `start(app, element)`") —
but it costs one extra sequential chunk fetch on a fresh deep link versus a
route-specific shell. Kit's own route-independent shell generator
(`builder.generateFallback`, `adapter-api.md`) produces a *root-layout-only*
branch with no page-specific preloads at all and would avoid this, but it is
only callable from inside a custom adapter's `adapt()` hook — out of scope
this sprint (no custom adapter is being written; see Open Questions). Verify
empirically with the deep-link Playwright scenario; if the extra waterfall
step causes an observable failure (it shouldn't — no scenario in this sprint
asserts request counts on `/items/[id]`), fall back to a hand-synthesized
shell per `csr-shell.md`'s "Recipe" HTML skeleton.

### Dev-mode wiring

`example/cmd/main.go` flags: `--listen` (default `:4173`, matching the
junkyard's validated `playwright.config.ts` convention), `--dev-proxy` (a URL;
when set, every request is proxied via `proxy.New`; when unset, the embedded
`static.NewHandler` serves the build). `paths.origin` is **not** read by Go at
runtime in dev mode — kit's dev middleware derives the request origin from the
incoming `Host` header, not from a build-time constant
(`dev-server.md` §"Vite config kit forces in dev" + "Go implementation
notes"/"Host header") — so no `ORIGIN` env var is needed for `vp dev`, only for
`vp build`.

`.air.toml` (root `air = "1.67.4"` via mise):
```toml
root = "."
tmp_dir = "tmp"

[build]
cmd = "go build -o ./tmp/skgo-example ./cmd"
bin = "tmp/skgo-example"
full_bin = "./tmp/skgo-example --listen :4173 --dev-proxy http://127.0.0.1:5173"
include_ext = ["go"]
exclude_dir = ["tmp", "web", "e2e"]
delay = 100

[misc]
clean_on_exit = true
```

`Procfile` (overmind `2.5.1` via mise, run from `example/`):
```
vite: cd web && mise x -- vp dev --port 5173 --strictPort
go: mise x -- air -c .air.toml
```

---

## Implementation Plan

### Step 1 — Repo scaffold
- `go.mod` (root, `module github.com/tylergannon/skgo`, `go 1.27`); root
  `mise.toml` pinning `go`, `air`, `overmind`.
- `example/go.mod` (`module github.com/tylergannon/skgo/example`, `go 1.27`,
  `require github.com/tylergannon/skgo v0.0.0`, `replace
  github.com/tylergannon/skgo => ../`).
- `example/web/package.json`, `vite.config.ts`, `tsconfig.json`,
  `example/web/mise.toml` (node `24.16.0`, `npm:vite-plus@0.3.0`) — see
  Dependencies for exact fields.
- `example/web/src/app.html`, `+layout.ts` (`ssr=false; prerender=true`),
  `+layout.svelte`, home `+page.svelte`, `items/[id]/+page.ts`
  (`prerender=false`), `items/[id]/+page.svelte`.
- `mise x -- vp install --frozen-lockfile` in `example/web/`; `mise x -- vp
  build` once by hand to confirm `_app/` and `prerendered/pages/index.html`
  exist before writing any Go.

### Step 2 — `static` package
- `static/gen/main.go` (`//go:build ignore`): parse
  `ephemeral/inspiration/reference/mrmime/deno/mod.ts`, emit
  `static/mimetable_gen.go`.
- `static/mimetable.go`: `func Lookup(name string) string` per mrmime's exact
  rule (`mrmime.md` §"Lookup rule") — trim, lowercase, substring after last
  `.`, empty string on miss (never fall back to `mime.TypeByExtension`).
- `static/static.go`: `Manifest`, `RouteMatcher`, `NewHandler`; content-hash
  ETags; the 4-step lookup order above.
- `static/static_test.go`: table tests seeded from a copied subset of
  `reference/sirv/tests/public/` (fünke.txt, `.hello`, `data.js.br` without a
  plain sibling — used only to prove Go correctly *ignores* compressed
  siblings this sprint, since precompression isn't implemented yet) plus a
  synthetic `_app/immutable/x.<hash>.js`, `_app/version.json`,
  `favicon.svg` tree; assert status/content-type/cache-control/etag per
  `sirv.md`'s "Recipes → Table-driven Go tests".

### Step 3 — `proxy` package
- `proxy/proxy.go`: `New(target *url.URL) http.Handler`.
- `proxy/proxy_test.go`: an `httptest.Server` standing in for `vp dev`
  asserting (a) GET passthrough preserves path/query/body, (b) `Host` header
  forwarding, (c) a WebSocket upgrade request (`Upgrade: websocket`) is
  proxied — use `httptest.NewServer` with a raw `net/http` `Hijacker` handler
  on the target side to simulate the HMR endpoint's 101 response, or
  `golang.org/x/net/websocket`/`nhooyr.io/websocket` purely in the test to
  drive a real upgrade handshake through the proxy (decide during
  implementation; avoid adding a websocket library to the production `proxy`
  package itself — Go's `httputil.ReverseProxy` needs none).

### Step 4 — `example/cmd/main.go`
- Flags `--listen`, `--dev-proxy`; dev-proxy branch → `proxy.New`; else →
  `fs.Sub(web.BuildFS, "build/client")` + `build/prerendered/pages/index.html`
  → `static.NewHandler` with the hardcoded `RouteMatcher` for `/` and
  `/items/{id}`.
- `example/web/dist.go`: the `//go:embed all:build/client
  all:build/prerendered` declaration.

### Step 5 — Dev loop
- `.air.toml`, `Procfile` as drafted above.
- Manual pass: `cd example && mise x -- overmind start`; confirm the browser
  at `:4173` shows the home page, HMR updates a `.svelte` edit, and editing
  `main.go` restarts the Go process (watch for the overmind log prefix
  change).

### Step 6 — `example/e2e` (playwright-bdd)
- `package.json`: `devDependencies: { "playwright-bdd": "9.2.0",
  "@playwright/test": "1.62.1" }`.
- `playwright.config.ts`: `defineBddConfig({ features: 'features/**/*.feature',
  steps: 'steps/**/*.ts' })`, `testDir` pointed at the generated directory
  per playwright-bdd's own convention, `use: { baseURL: process.env.BASE_URL }`
  — **verify the exact `defineBddConfig`/`createBdd` API against the installed
  9.2.0 package** (not in the semantic index; the index only covers kit,
  devalue, sirv, mrmime, polytype and the junkyard — playwright-bdd is a new
  dependency for this sprint with no pinned prior art here).
- Four features translating the intent's success criteria into Gherkin:
  `home.feature` ("the home page renders a Svelte component"),
  `navigation.feature` ("clicking a link navigates without a full page
  reload" — port the request-counting idiom from
  `sources/junkyard/app/e2e-suite.md`'s helper `recordRequests`/
  `waitForHydration`, reimplemented minimally since this sprint has no
  hydration marker element to wait on — add one, e.g. `data-testid="started"`
  set in a root `$effect`, mirroring `sources/kit/test-apps/harness-helpers.md`'s
  `body.started` convention), `deep-link.feature` ("requesting /items/42
  directly renders Item #42"), `unknown-route.feature` ("requesting an unknown
  path returns 404 and renders the app's error page").
- `steps/common.ts`: Given/When/Then implementations using `@playwright/test`'s
  `expect`.

### Step 7 — Go-driven proof harness
- `example/e2e_test.go` (`//go:build e2e`): two subtests, `TestProdMode` and
  `TestDevMode`.
  - `TestProdMode`: `os/exec` — `ORIGIN=http://127.0.0.1:4173 mise x -- vp
    build` (cwd `example/web`), `go build -o <tmp>/skgo-example ./cmd` (cwd
    `example`), start the binary, poll `GET /` until it answers, set
    `BASE_URL`, run `mise x -- node node_modules/playwright-bdd/dist/cli/*
    gen` then `mise x -- node node_modules/@playwright/test/cli.js test` (cwd
    `example/e2e`) — mirroring the junkyard's validated invocation style from
    `sources/junkyard/app/toolchain.md` (call the CLI `.js` file directly, not
    `npx`/`pnpm exec`, because `vp install`'s `devEngines` pin makes `npm`/
    `npx` refuse to run in the project). Fail the Go test on any non-zero
    exit; also run `--list --reporter=json` first and assert the expected
    test count, porting the idea (not the shell script) from
    `check-level.sh`'s exact-count gate per `toolchain.md`'s "Reusable
    verdicts".
  - `TestDevMode`: start `vp dev --port 5173` and the Go binary with
    `--dev-proxy http://127.0.0.1:5173 --listen :4174`, run the same feature
    suite against `BASE_URL=http://127.0.0.1:4174`.
- `go test -tags e2e ./example/...` is the CI gate for this sprint;
  plain `go test ./...` (no tag) stays fast and Node-free, covering `proxy`
  and `static` only.

### Step 8 — Validation pass
- `go vet ./...` clean at root and in `example/`.
- Manual: `curl -I http://127.0.0.1:4173/about/nope` (or equivalent) → confirm
  404 with shell body vs `curl -I .../_app/immutable/nope.js` → 404 empty body
  with `no-store`.
- Record any deviation from this plan (especially the modulepreload gap and
  the HMR-through-`vp`-specifically claim) in the worklog per
  `agent-protocol`.

---

## Files Summary

| Path | Purpose |
|---|---|
| `go.mod`, `mise.toml` | Root module + toolchain pins (go, air, overmind) |
| `proxy/proxy.go`, `proxy_test.go` | Dev reverse proxy incl. WebSocket upgrade |
| `static/static.go`, `mimetable.go`, `mimetable_gen.go`, `gen/main.go`, `static_test.go`, `testdata/` | Embedded-build static + CSR shell handler, generated MIME table |
| `example/go.mod` | Example module, `replace` to root |
| `example/cmd/main.go` | Flags, mode switch, wiring |
| `example/web/dist.go` | `//go:embed all:build/client all:build/prerendered` |
| `example/web/package.json`, `vite.config.ts`, `tsconfig.json`, `mise.toml` | Kit-3 app config + JS toolchain pins |
| `example/web/src/app.html`, `routes/+layout.ts`, `routes/+layout.svelte`, `routes/+page.svelte`, `routes/items/[id]/+page.ts`, `routes/items/[id]/+page.svelte`, `static/favicon.svg` | The app itself |
| `example/e2e/package.json`, `playwright.config.ts`, `features/*.feature`, `steps/common.ts` | playwright-bdd proof suite |
| `example/e2e_test.go` | Go-driven orchestration of both modes' Playwright runs |
| `example/Procfile`, `.air.toml` | Dev process supervision (overmind + Air) |

---

## Definition of Done

1. `cd example && mise x -- overmind start` serves the app at the Go port;
   client navigation between `/` and `/items/*` does not reload the document;
   editing a `.svelte` file hot-updates; editing a `.go` file restarts the Go
   process (observed manually, not asserted by an automated test this
   sprint — dev-mode HMR is a developer-experience property, not something
   `playwright-bdd` needs to assert; the dev-mode *serving* behavior itself
   (Use Case 1's "browser shows the app", deep link, unknown route) **is**
   asserted by `TestDevMode`).
2. `ORIGIN=http://127.0.0.1:4173 mise x -- vp build` (in `example/web`) then
   `go build ./example/cmd` then running the binary with Vite stopped serves
   the identical app from the embedded filesystem to a real browser.
3. `go test -tags e2e ./example/...` passes both `TestProdMode` and
   `TestDevMode`, each running the four `playwright-bdd` features with no
   unexpectedly-skipped scenario.
4. `go test ./...` (no tag) and `go vet ./...` pass at the repo root and in
   `example/`, covering `proxy` and `static` in isolation (no Node/Playwright
   dependency).
5. `//go:embed all:...` is verified to include `_app/**` (a unit test that
   asserts `fs.Stat` succeeds for a known `_app/immutable/...` path, not just
   manual inspection) — this trap is easy to reintroduce on refactor.
6. AGENTS.md's outdated design description is **not** silently left
   contradicting this sprint's shipped code; if not rewritten this sprint,
   the open question is recorded (see below), not ignored.

---

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `//go:embed build/client build/prerendered` (no `all:`) silently drops `_app/**` — a 404 storm with no build-time signal. | Use `all:` prefix; add the DoD #5 unit test asserting a known `_app/immutable/...` path is present in the embedded FS. |
| `embed.FS` reports zero `ModTime`; a naive sirv port would emit a constant, potentially colliding ETag across rebuilds of same-size files. | Content-hash (SHA-256) ETags computed once at startup, documented as an intentional deviation from `sirv.md`'s size+mtime formula. |
| The single prerendered `/` shell's modulepreload list doesn't match `/items/[id]`'s actual imports — extra lazy-chunk round trip on deep links. | Documented gap (see Architecture); verify empirically via the deep-link feature; fall back to a hand-synthesized shell (`csr-shell.md` recipe) only if a scenario actually fails. |
| HMR websocket-through-the-Go-proxy behavior is asserted from general Vite knowledge, not pinned kit source (`dev-server.md`'s own caveat). | Manual verification in Step 5 before trusting it in the e2e suite; if `vp dev`'s HMR path differs from stock Vite, treat as a new finding for the worklog, not a silent guess. |
| `playwright-bdd` 9.2.0's exact config/API shape isn't in the semantic index (only kit/devalue/sirv/mrmime/polytype/junkyard are indexed). | Verify against the installed package's own types/docs during Step 6 rather than the model's general knowledge; keep the Go orchestration test resilient to CLI-invocation details by asserting on process exit code + reported test count, not on internal file paths. |
| `npm`/`npx` refuse to run once `vp install` pins `devEngines` (kit-3 fact, `toolchain.md`). | Every JS CLI invocation in `e2e_test.go`, `.air.toml`, and `Procfile` goes through `mise x -- node node_modules/<pkg>/...cli.js`, never `npx`/`pnpm exec`. |
| `paths.origin` mismatch between the build and Go's actual listen address would 403 any future mutating request; this sprint has none, so the failure mode is currently latent. | Build with `ORIGIN` set to the exact prod `--listen` address (`4173`) now, even though nothing exercises CSRF yet, so Phase B doesn't inherit a silent mismatch. |
| Root's `mise.toml` (go/air/overmind) and `example/web/mise.toml` (node/vite-plus) both apply when working inside `example/web/` — mise config layering could surprise a contributor. | Document the two-file split explicitly in a short `example/README` note (not this plan) rather than relying on tribal knowledge. |

---

## Dependencies

**Go** (root `go.mod`, `go 1.27`; confirmed installed: `go1.27.1 darwin/arm64`):
no external modules required for `proxy`/`static` (stdlib only: `net/http`,
`net/http/httputil`, `embed`, `io/fs`, `crypto/sha256`, `regexp`).

**Go** (`example/go.mod`, `go 1.27`): `require github.com/tylergannon/skgo
v0.0.0` with `replace github.com/tylergannon/skgo => ../`.

**Node/JS** (`example/web/package.json`, all exact pins, junkyard-validated —
`sources/junkyard/app/toolchain.md`):
- `@sveltejs/kit` `3.0.0-next.25`
- `svelte` `5.56.10`
- `vite` `8.2.2`
- `@sveltejs/vite-plugin-svelte` `7.3.0`
- `@sveltejs/adapter-node` `6.0.0-next.10` (build vehicle only)
- `typescript` `6.0.3`
- `@types/node` `26.4.1`

**Node/JS** (`example/e2e/package.json`, new this sprint, confirmed current on
npm as of 2026-09-05):
- `playwright-bdd` `9.2.0`
- `@playwright/test` `1.62.1` (pinned to the junkyard's validated version, not
  the newest `1.63.0`, for continuity with the ported acceptance-suite idioms;
  `playwright-bdd@9.2.0` declares `peerDependencies: { "@playwright/test":
  ">=1.44" }`, so `1.62.1` is compatible)

**mise-provisioned tools** (confirmed resolvable via `mise ls-remote`/`gh
release list` on 2026-09-05):
- `go` `1.27.1` (root `mise.toml`)
- `air-verse/air` `v1.67.4` via `aqua:air-verse/air` (root `mise.toml`)
- `DarthSim/overmind` `v2.5.1` via `go:github.com/DarthSim/overmind/v2` (root
  `mise.toml`)
- `node` `24.16.0`, `npm:vite-plus` `0.3.0` (`example/web/mise.toml`,
  unchanged from the junkyard)

---

## Open Questions

1. **Modulepreload fidelity** (carried from the intent): is the `/`
   prerendered shell's boot behavior for `/items/[id]` observably identical to
   what `builder.generateFallback` would produce, for the purposes of this
   sprint's scenarios? Verify with the deep-link feature; only invest in a
   hand-synthesized shell if it actually fails.
2. **Exact-count gate mechanics**: should the "assert no test was silently
   skipped" check (ported from the junkyard's `check-level.sh` idea) live
   inside `e2e_test.go`, or as a `playwright-bdd`/Playwright reporter
   assertion? Decide during Step 7; either is acceptable, but pick one so it
   isn't duplicated.
3. **WebSocket test technique** for `proxy_test.go`: hand-roll a minimal
   HTTP/1.1 upgrade handshake with `net/http`'s `Hijacker`, or take a small
   test-only WebSocket dependency? Either is fine; avoid adding the dependency
   to the shipped `proxy` package itself.
4. **AGENTS.md rewrite**: still describes the old adapter+sidecar+SSR design.
   Not touched by this sprint's code; the build plan flagged
   `ephemeral/plans/AGENTS.md.proposed` as awaiting Tyler's approval — confirm
   whether that rewrite happens before or after this sprint merges, so
   contributors reading AGENTS.md mid-sprint aren't misled.
5. **Graceful shutdown**: `example/cmd/main.go`'s `http.Server` — is bare
   `ListenAndServe` (no `signal.NotifyContext`/graceful drain) acceptable for
   this sprint, given Air/overmind just kill and restart the process on every
   change? Proposed answer: yes, defer graceful shutdown to whichever later
   sprint cares about connection draining in a real deployment; flag if that
   assumption is wrong.
6. **`playwright-bdd` output directory conventions**: not verified against the
   installed package (outside the semantic index); confirm the generated
   test directory / `defineBddConfig` shape against `node_modules/playwright-bdd`
   directly at the start of Step 6 rather than assuming this plan's sketch is
   exact.
