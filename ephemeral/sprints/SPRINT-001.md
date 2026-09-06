# Sprint 001: Bare Go server fronting a SvelteKit app

## Pyramid Index

- L0: One Go binary serves a real SvelteKit 3 app — in dev by proxying the Vite dev server (HMR included) under Air and overmind, in prod from an embedded build produced by a 20-line skgo adapter — proven by playwright-bdd scenarios run against both modes.
- L1:
  - Repo shape from the brief: root `go.mod` + `go.work`, `example/` module with `cmd/`, `web/`, `e2e/`.
  - Two library handlers: `NewDevProxy` (reverse proxy incl. WebSocket upgrade) and `NewStaticHandler` (embedded build + boot document for page routes).
  - A thin skgo adapter writes `build/client`, `build/index.html` (kit's own `generateFallback`) and `build/skgo.manifest.json`; no Node server output exists.
  - Example app: root layout with `ssr = false`, routes `/`, `/about`, `/items/[id]`, one shared Svelte component; no remote functions, no server loads.
  - Proof: three Gherkin scenarios via playwright-bdd against `BASE_URL`, dev and prod; Go unit tests for both handlers; embed guard test.
  - Built as specified except where source contradicted the plan: `vp dev` needs the project-local Vite+ with `vite` aliased to `@voidzero-dev/vite-plus-core` (kit's `instanceof` check), `builder.writeJson` does not exist, Air's `root` is not its working directory, and `go test ./...` does not cross module boundaries. Each is diagnosed in `ephemeral/worklog/202609052129-sprint-001-execute.md`; the Architecture and Implementation Plan sections below are the pre-execution spec and still carry those four errors.
- L2:
  - Adapter and build contract → §Architecture / "Build output"; kit facts in `ephemeral/semantic-index/sources/kit/build-adapt/adapter-api.md`, `spa-and-prerender.md`.
  - Proxy and HMR → §Architecture / "Dev mode"; `sources/kit/build-adapt/dev-server.md`.
  - Static handler → §Architecture / "Prod mode"; `sources/kit/server-runtime/csr-shell.md`, `sources/kit/client-router/csr-boot.md`.
  - Toolchain pins and launch lines → §Dependencies; `sources/junkyard/app/toolchain.md`, `sources/kit/docs/kit3-facts.md`.
  - Scenarios → §Implementation Plan / Phase 5; `sources/junkyard/app/e2e-suite.md`.

## Overview

The first runnable slice of skgo. A browser talks only to Go. In development Go forwards every request and WebSocket upgrade to `vp dev`; Air rebuilds Go on `.go` changes; overmind runs both from one Procfile. In production `vp build` runs the skgo adapter, which writes the client bundle, the boot document, and a JSON manifest into `example/web/build/`; `go build ./example/cmd` embeds that directory; the binary serves it with no Node process anywhere. Playwright scenarios prove real Svelte components render through Go in both modes.

Out of scope: remote functions, server loads and `__data.json`, code generation, the project generator, static-serving parity with adapter-node (compression, sirv rules, MIME table), any harness beyond `go test` and the Playwright CLI.

## Use Cases

1. **Dev loop.** `cd example && overmind start`. Browser at `http://127.0.0.1:8080/` shows the home page; clicking to `/about` and `/items/42` does not reload the document; editing `Greeting.svelte` hot-updates over a WebSocket that terminates at port 8080; editing a `.go` file makes Air rebuild and restart Go.
2. **Prod binary.** `cd example/web && mise x -- vp build`, then `cd .. && go build -o tmp/skgo-example ./cmd`, stop Vite, run `./tmp/skgo-example`. The same pages work from the embedded build.
3. **Deep link.** A fresh browser tab opens `/items/42` directly; Go serves the boot document for that route (200); kit's client router renders the item page from the URL.
4. **Unknown path.** `/nope` gets the boot document with status 404; kit's client renders `+error.svelte`.

## Architecture

### Repository layout
```
go.work                      use . ./example
go.mod                       module github.com/tylergannon/skgo   (go 1.27)
proxy.go  proxy_test.go      NewDevProxy
static.go static_test.go     NewStaticHandler
example/
  go.mod                     module github.com/tylergannon/skgo/example; replace ../
  mise.toml                  node 24.16.0, "npm:vite-plus" 0.3.0
  Procfile                   overmind
  .air.toml                  Air
  cmd/main.go                --listen, --proxy; X-Skgo-Mode header
  web/
    skgo-adapter.js          the adapter (below)
    vite.config.ts  tsconfig.json  package.json  pnpm-workspace.yaml
    dist.go                  package web; //go:embed all:build
    dist_test.go             asserts build/index.html and a build/client/_app/immutable/ file exist
    src/app.html
    src/lib/Greeting.svelte
    src/routes/+layout.ts    export const ssr = false;
    src/routes/+layout.svelte  nav links to /, /about, /items/42
    src/routes/+page.svelte  <Greeting/>
    src/routes/about/+page.svelte
    src/routes/items/[id]/+page.svelte   let { params } = $props()
    src/routes/+error.svelte
  e2e/
    package.json  playwright.config.ts  tsconfig.json
    features/app.feature     three scenarios
    steps/app.ts
```

### Build output (the adapter)
`example/web/skgo-adapter.js`, referenced from `vite.config.ts` as `adapter: skgo()`:
```js
export default function skgo({ out = 'build' } = {}) {
  return {
    name: 'skgo',
    async adapt(builder) {
      builder.rimraf(out);
      builder.writeClient(`${out}/client`);
      builder.writePrerendered(`${out}/prerendered`);
      await builder.generateFallback(`${out}/index.html`);
      builder.writeJson(`${out}/skgo.manifest.json`, {
        appDir: builder.config.appDir,
        base: builder.config.paths.base,
        version: builder.config.version.name,
        routes: builder.routes.map((r) => ({ id: r.id, pattern: r.pattern.source }))
      });
    }
  };
}
```
Facts this rests on (pinned kit 3.0.0-next.25): `generateFallback` renders the boot document through kit's fallback path, which executes no loads and has root-layout-only preloads (`core/adapt/builder.js:L141-L160`, `core/postbuild/fallback.js`); `builder.routes` exposes `id` and `pattern` (`types/index.d.ts`, `RouteDefinition`); `writeClient` copies `_app/` and `static/`. The document's asset URLs are absolute because fallback mode ignores `paths.relative`. No route is prerendered this sprint (nothing sets `prerender = true`), so `build/prerendered/` is empty and the call is kept only for later. Kit's Vite plugin still builds the server bundle into `.svelte-kit/output/server/` during `vp build`; the adapter never copies it, and it never runs.

`vite.config.ts`:
```ts
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import skgo from './skgo-adapter.js';
export default defineConfig({
  plugins: [sveltekit({ adapter: skgo(), paths: { origin: process.env.ORIGIN ?? 'http://127.0.0.1:8080' } })]
});
```
No `svelte.config.js` (kit 3 refuses to run with one). `tsconfig.json` extends `$app/tsconfig`. `package.json` carries the `#lib` imports map and `devEngines` pnpm pin exactly as in `sources/junkyard/app/toolchain.md`.

### Embed
`example/web/dist.go`:
```go
package web
import "embed"
//go:embed all:build
var Build embed.FS
```
`all:` is mandatory: without it Go silently omits `_app/` (underscore-prefixed). A fresh checkout does not compile `example/...` until `vp build` has run; that is deliberate — a missing frontend fails loudly. `dist_test.go` asserts `build/index.html`, `build/skgo.manifest.json`, and at least one `build/client/_app/immutable/**` file are present.

### Prod mode: `skgo.NewStaticHandler(build fs.FS) (http.Handler, error)`
Reads `index.html` and `skgo.manifest.json` at construction (error if missing). Per GET/HEAD request, path cleaned and validated (`fs.ValidPath`):
1. Exact file under `client/` → serve. Under `/<appDir>/immutable/` add `Cache-Control: public, max-age=31536000, immutable`; every file gets a content-hash ETag (`"<sha256-prefix>"`) and answers `If-None-Match` with 304.
2. Any other miss under `/<appDir>/` → 404, empty body, never the document.
3. Path matches a manifest route pattern (Go `regexp` over `pattern`; kit's patterns are RE2-compatible) → `index.html`, 200, `Content-Type: text/html; charset=utf-8`.
4. Otherwise → `index.html`, 404.
Other methods → 405 with `Allow: GET, HEAD`. Content types: explicit map for `.js .mjs .css .html .json .svg .webmanifest .map .ico .png .woff2 .txt`, then `mime.TypeByExtension`. Nothing else (no compression, no ranges beyond `http.ServeContent`, no prerendered lookup beyond exact files).

### Dev mode: `skgo.NewDevProxy(target *url.URL) http.Handler`
`httputil.ReverseProxy` with `Rewrite: func(p *httputil.ProxyRequest) { p.SetURL(target); p.Out.Host = p.In.Host; p.SetXForwarded() }` and an `ErrorHandler` returning 502. The standard library tunnels HTTP/1.1 upgrades, so the HMR WebSocket needs nothing extra. Vite 8.2.2's client opens the HMR socket on the page's own port (`__HMR_PORT__` is null by default), so it lands on Go. Vite also has a direct-target fallback to 5173; the dev check must observe the upgrade at Go (log line `ws upgrade /` in the proxy at debug level), not just a hot-updating page. The inbound `Host` is `127.0.0.1:8080`, a loopback host Vite accepts without `server.allowedHosts`.

### `example/cmd/main.go`
Flags: `--listen` (default `127.0.0.1:8080`), `--proxy` (URL; empty = embedded mode). Wraps the chosen handler to set `X-Skgo-Mode: dev|prod`. Plain `http.Server`; no graceful shutdown this sprint.

### Dev process model
`example/Procfile`:
```
web: cd web && mise x -- node_modules/.bin/vp dev --host 127.0.0.1 --port 5173 --strictPort
go:  air -c .air.toml
```
`example/.air.toml` (root is the repo root so edits to the library also rebuild):
```toml
root = ".."
tmp_dir = "example/tmp"
[build]
  cmd = "cd example && go build -o tmp/skgo-example ./cmd"
  bin = "example/tmp/skgo-example"
  args_bin = ["--listen", "127.0.0.1:8080", "--proxy", "http://127.0.0.1:5173"]
  include_ext = ["go"]
  exclude_dir = ["example/web", "example/e2e", "example/tmp", "ephemeral", ".claude", ".agents", ".git"]
  delay = 200
```

## Implementation Plan

### Phase 1 — Skeleton and toolchain
Files: `go.work`, `go.mod`, `example/go.mod`, `example/mise.toml`, `example/web/{package.json,pnpm-workspace.yaml,tsconfig.json,vite.config.ts,skgo-adapter.js}`, `example/web/src/**`, `.gitignore` (`example/web/build/`, `example/tmp/`, `node_modules/`, `.svelte-kit/`, `example/e2e/.features-gen/`, `test-results/`).
1. Copy pins from `sources/junkyard/app/toolchain.md` (kit 3.0.0-next.25, svelte 5.56.10, vite 8.2.2, vite-plugin-svelte 7.3.0, typescript 6.0.3, @types/node 26.4.1); drop adapter-node.
2. `cd example && mise install && cd web && mise x -- vp install`; commit the lockfile.
3. Author the app: layout with nav, `Greeting.svelte` imported via `#lib/Greeting.svelte`, three pages, `+error.svelte`, root `+layout.ts` with `export const ssr = false;`.
4. `ORIGIN=http://127.0.0.1:8080 mise x -- vp build`; confirm `build/index.html`, `build/skgo.manifest.json`, `build/client/_app/immutable/**`. Inspect `index.html`: absolute `/_app/...` URLs, `kit.start(app, element)` boot.

### Phase 2 — Library, test-first (`go test ./...`, no Node)
1. `static_test.go` with `testing/fstest.MapFS`: exact client file; immutable header; `_app` miss → 404 empty; known route → 200 + document; unknown → 404 + document; ETag/304; HEAD; 405; traversal rejected; constructor errors when `index.html`/manifest missing.
2. `static.go` per the spec above.
3. `proxy_test.go`: method/path/query/body/Host forwarded to an `httptest.Server`; `X-Forwarded-*` present; a hijacking upstream returning `101 Switching Protocols` echoes bytes through the proxy; unreachable target → 502.
4. `proxy.go` per the spec above.

### Phase 3 — Example command and embed
1. `example/web/dist.go`, `dist_test.go` (needs Phase 1 step 4 output).
2. `example/cmd/main.go`.
3. With Vite stopped: `go build -o example/tmp/skgo-example ./example/cmd` from the root (via `go.work`), run it, `curl -i` `/`, `/items/42`, `/nope`, `/_app/version.json`, one immutable chunk, one missing immutable chunk; check status, `X-Skgo-Mode: prod`, cache headers.

### Phase 4 — Dev loop
1. `example/Procfile`, `example/.air.toml`.
2. `cd example && overmind start`. Verify in the browser: document URL on 8080; `X-Skgo-Mode: dev` on the document; HMR socket on 8080 (proxy debug log shows the upgrade); edit `Greeting.svelte` → updates without reload; touch a `.go` file → Air rebuilds, PID changes, page reloads fine.

### Phase 5 — playwright-bdd
Files: `example/e2e/package.json` (`@playwright/test` 1.63.0, `playwright-bdd` 9.2.0, `typescript` 6.0.3), `playwright.config.ts` (`defineBddConfig({ features: 'features/**/*.feature', steps: 'steps/**/*.ts' })`, `baseURL: process.env.BASE_URL`, one Chromium project, `workers: 1`, `retries: 0`, trace on failure, no `webServer`), `features/app.feature`, `steps/app.ts`.
Verify the `defineBddConfig`/`createBdd` API against the installed package before writing steps; it is not in the semantic index.
Scenarios:
```gherkin
Feature: SvelteKit served by Go
  Scenario: Home page renders a Svelte component through Go
    Given I open "/"
    Then the document response came from skgo in the expected mode
    And I see the greeting component
  Scenario: Client-side navigation does not reload the document
    Given I open "/"
    When I click the link to "/items/42"
    Then I see "Item 42"
    And exactly 1 document request was made
  Scenario: Deep link to a dynamic route
    Given I open "/items/7"
    Then I see "Item 7"
```
Steps: `page.goto`, capture the `document` response and assert `x-skgo-mode` equals `EXPECTED_MODE`; count `request.resourceType() === 'document'` via a per-scenario fixture; text assertions with `expect(locator).toHaveText`.
Install: `cd example/e2e && mise x -- vp install && mise x -- node node_modules/@playwright/test/cli.js install chromium`.
Run (dev stack up):
```sh
cd example/e2e && BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=dev mise x -- node node_modules/playwright-bdd/dist/cli/index.js && BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=dev mise x -- node node_modules/@playwright/test/cli.js test
```
Run (prod: overmind stopped, binary from Phase 3 running): same with `EXPECTED_MODE=prod`.

### Phase 6 — Gates
`go test ./...` and `go vet ./...` from the root (workspace covers `example/`), both Playwright runs green, worklog entry for anything that deviated.

## Files Summary
| Path | Purpose |
|---|---|
| `go.work`, `go.mod`, `example/go.mod` | modules; root builds of `./example/cmd` |
| `proxy.go`, `proxy_test.go` | `NewDevProxy` |
| `static.go`, `static_test.go` | `NewStaticHandler` |
| `example/cmd/main.go` | flags, mode header, server |
| `example/web/skgo-adapter.js` | the adapter (client + boot document + manifest) |
| `example/web/dist.go`, `dist_test.go` | `//go:embed all:build` and its guard |
| `example/web/{package.json,pnpm-workspace.yaml,tsconfig.json,vite.config.ts,src/**}` | the kit app |
| `example/mise.toml`, `example/Procfile`, `example/.air.toml` | toolchain and dev loop |
| `example/e2e/**` | playwright-bdd package |
| `.gitignore` | build outputs, deps, generated tests |

## Definition of Done
1. `cd example && overmind start`; browser at `http://127.0.0.1:8080/` renders the app; `.svelte` edit hot-updates with the HMR socket on 8080; `.go` edit restarts Go via Air.
2. `vp build` then `go build ./example/cmd` from the root; with Vite stopped the binary serves `/`, `/about`, `/items/42` (deep link), and `/nope` (404 + kit error page) from the embedded FS; document responses carry `X-Skgo-Mode: prod`.
3. The three playwright-bdd scenarios pass against dev and against prod.
4. `go test ./...` and `go vet ./...` pass from the root; `dist_test.go` proves `_app/immutable` is embedded.
5. `example/web/build/` contains no Node server code; `skgo-adapter.js` is the only JavaScript outside the app and tests.

## Risks & Mitigations
| Risk | Mitigation |
|---|---|
| `generateFallback` document fails to boot a deep link | It is kit's own SPA document; if it fails, inspect against `sources/kit/server-runtime/csr-shell.md`; do not hand-write the boot script |
| `//go:embed` without `all:` drops `_app/` silently | `all:` + `dist_test.go` |
| Fresh checkout cannot `go build example/` | Documented order: `vp build` first; loud failure is the intended policy |
| HMR appears to work but bypassed Go via Vite's direct target | Verify the upgrade at Go's port (debug log) in Phase 4 |
| `paths.origin` mismatch → future CSRF 403s | Build with `ORIGIN` = the prod listen origin now, even though nothing posts yet |
| `npm`/`npx` refuse to run after `vp install` (devEngines) | All JS CLIs run as `mise x -- node node_modules/<pkg>/…` |
| playwright-bdd API differs from the sketch | Check the installed package first; keep steps small |
| Air `root=".."` watches too much | `exclude_dir` list above; adjust if rebuild storms appear |

## Dependencies
Go 1.27.1; Air 1.67.x and overmind 2.5.1 (installed system-wide; optional mise pins); Node 24.16.0 and vite-plus 0.3.0 via `example/mise.toml`; pnpm 11.25.0 via `devEngines`; `@sveltejs/kit` 3.0.0-next.25, `svelte` 5.56.10, `vite` 8.2.2, `@sveltejs/vite-plugin-svelte` 7.3.0, `typescript` 6.0.3, `@types/node` 26.4.1; `@playwright/test` 1.63.0, `playwright-bdd` 9.2.0; Chromium via Playwright.

## Open Questions
1. Should the adapter live in the skgo repo as a published-later package rather than `example/web/`? Fine in `example/web/` this sprint; move when the project generator needs it.
2. Does `builder.routes[].pattern.source` need any translation for Go `regexp`? Expect none; the first `vp build` shows the real patterns.
3. `writePrerendered` is called for future opt-in prerendered static pages; confirm an empty `build/prerendered/` does not upset `//go:embed all:build` (it will not exist if empty — the embed pattern is the parent `build`, so fine).
