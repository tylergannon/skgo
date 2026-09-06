# Sprint 001: Bare Go server fronting a SvelteKit app

## Pyramid Index

- **L0:** Produce the first working skgo server: one Go binary serves a real CSR-only SvelteKit 3 application, proxies the complete Vite+ development surface (including HMR), and embeds the adapter-node client/prerendered production output.
- **L1:**
  - Establish the two-module Go workspace and exact, reproducible SvelteKit/Vite+/Air/overmind/Playwright-BDD toolchain.
  - Add two public `http.Handler` constructors: a transparent development reverse proxy and a production static/SPA handler over `fs.FS`.
  - Build a small SvelteKit app with `/`, `/items/[id]`, and a custom error page; Kit supplies the browser router and boot document, while Go owns the public socket in both modes.
  - Make adapter-node write `build/client` and `build/prerendered`; embed only those directories in the example binary after `vp build`.
  - Demonstrate the result in Chromium against the Go port in dev and prod, plus unit-test HTTP forwarding, WebSocket upgrades, fallback routing, cache policy, and invalid requests.
- **L2:**
  - Repository/toolchain details: [Dependencies](#dependencies), Phase 1, and [Files Summary](#files-summary).
  - Handler contracts and request flow: [Architecture](#architecture), Phases 2-3.
  - Kit application and build/embed contract: [Architecture](#production-build-and-embed-contract), Phase 4.
  - Air, overmind, Vite+, and HMR: [Development-process-model](#development-process-model), Phase 5.
  - Browser and command-level acceptance: Phase 6 and [Definition of Done](#definition-of-done).

## Overview

This sprint creates the smallest honest vertical slice of skgo. A browser connects only to the Go server at `http://127.0.0.1:8080`. In development, Go transparently proxies every request and WebSocket upgrade to `vp dev` at `http://127.0.0.1:5173`; Air rebuilds/restarts Go and overmind supervises both processes. In production, no Node process runs: the Go binary serves the client and prerendered trees produced by Kit and embedded at compile time.

The sprint deliberately does **not** implement remote functions, server loads, code generation, Go route discovery, SSR, a project generator, or adapter parity. It creates the runnable foundation those later features can extend.

The intent's Context supersedes the old design in `AGENTS.md`: skgo is a Go backend for CSR-only SvelteKit, not a SvelteKit adapter backed by a production Node sidecar. The still-applicable repository rules remain: Go owns skgo behavior, JavaScript/TypeScript is limited to Kit and browser-test surfaces, verification uses ordinary Go tests and the real application, and no proof framework is introduced.

Semantic-index route A yields five constraints that shape this plan:

1. Kit 3's CSR boot document is subtle; do not synthesize it first. Let Kit generate `/` as a prerendered `ssr = false` document containing the versioned payload, preload links, and `kit.init(...); kit.start(...)` sequence.
2. `paths.relative: false` is required so the same root document can boot deep links with absolute `/_app/...` asset URLs.
3. Adapter-node writes static/public assets and hashed client chunks to `build/client`, and prerendered HTML to `build/prerendered`; its generated Node server is irrelevant and must not be embedded.
4. A dev proxy should intercept nothing in this sprint. Page HTML, Vite transforms, `/@vite/*`, `/@fs/*`, static files, and the HMR WebSocket all go to the Vite+ target.
5. Production must distinguish missing `/_app/immutable/*` assets from page routes: a missing hashed asset is a 404, never the SPA document.

## Use Cases

### UC1: Start the complete development loop

After one dependency install and one initial frontend build, a developer runs `overmind start` from `example/`. Overmind starts `vp dev` and Air. Visiting `http://127.0.0.1:8080/` renders the Svelte component through Go. Editing a `.svelte` file updates the page through the proxied HMR WebSocket without a document reload; editing a `.go` file causes Air to rebuild and restart the Go process.

### UC2: Navigate entirely in the browser

The home page links to `/items/42`. Clicking it uses Kit's client router without a second document request. Loading `/items/42` directly also succeeds because Go serves the Kit-generated boot document and the client router resolves the dynamic route.

### UC3: Render Kit's error UI for an unknown route

Loading `/not-a-route` returns the boot document from Go. Kit's client router finds no route and renders the app's `+error.svelte`. No Node server participates.

### UC4: Build and run a standalone production binary

With Vite stopped, a developer runs `vp build`, then `go build ./example/cmd`, then starts the binary. The same browser scenarios pass against the Go port. The executable contains `build/client` and `build/prerendered`, but not adapter-node's generated server runtime.

### UC5: Detect a missing frontend build immediately

On a clean checkout where `example/web/build/client` or `example/web/build/prerendered` does not exist, compilation of `example/web/assets.go` fails at the `go:embed` directive. The remedy is explicit: run the pinned install/build commands. This sprint does not check in placeholders and does not silently embed stale fallback content.

## Architecture

### Repository and module boundary

Use a Go workspace so the root command required by the intent works despite `example/` being a separate module:

```text
go.work                              use . and ./example
go.mod                               module github.com/tylergannon/skgo
proxy.go                             development handler
static.go                            production handler
example/go.mod                       module github.com/tylergannon/skgo/example
example/cmd/main.go                  application flags and socket ownership
example/web/                         Kit/Vite root and embedded build package
example/e2e/                         independent Playwright-BDD package
```

Both Go modules declare `go 1.27.1`. `example/go.mod` requires `github.com/tylergannon/skgo v0.0.0` and uses `replace github.com/tylergannon/skgo => ..` so it also builds outside workspace mode. There are no third-party Go runtime dependencies in this sprint.

### Public Go API

The root package exposes only the reusable handler boundary:

```go
package skgo

type StaticOptions struct {
	Client      fs.FS
	Prerendered fs.FS
	Document    string
}

func NewDevProxy(target *url.URL) (http.Handler, error)
func NewStaticHandler(options StaticOptions) (http.Handler, error)
```

`NewDevProxy` accepts only an absolute `http` or `https` URL with no query or fragment. It returns an `httputil.ReverseProxy` configured through `Rewrite`: `ProxyRequest.SetURL(target)` selects the upstream; the inbound `Host` is restored on `Out.Host` so Kit observes the public Go origin; `ProxyRequest.SetXForwarded()` supplies standard forwarding headers. Its `ErrorHandler` returns 502. The standard library proxy owns HTTP/1.1 upgrade tunneling; a raw upgrade test locks that behavior down.

`NewStaticHandler` validates non-nil filesystems and verifies `Document` (for this example, `index.html`) exists in `Prerendered` during construction. It implements this order for `GET` and `HEAD`:

1. Normalize the URL path to an `fs.ValidPath`-compatible name and reject traversal or malformed paths.
2. Serve an exact regular file from `Client`.
3. Serve an exact prerendered candidate: `/` to `index.html`, `/x` to `x.html`, and `/x/` to `x/index.html` when present.
4. Return 404 with `Cache-Control: no-store` for any missing `/_app/immutable/*`; never use the document fallback there.
5. For a request accepting `text/html`, serve `Prerendered/Document`; this supports dynamic deep links and lets Kit render unknown-route UI.
6. Return 404 for non-document misses. Return 405 with `Allow: GET, HEAD` for other methods.

The handler emits extension-correct content types (explicitly `.js`/`.mjs` as `text/javascript; charset=utf-8`, `.html` as `text/html; charset=utf-8`, and standard-library MIME lookup for the rest), strong content-derived ETags, `304 Not Modified` for matching `If-None-Match`, and these cache rules:

- `/_app/immutable/**`: `public,max-age=31536000,immutable` on 200.
- `/_app/version.json` and `/_app/manifest.js`: `no-cache`.
- HTML documents and other public assets: `no-cache`.

Precompressed `.br`/`.gz` negotiation and full sirv/mrmime parity are explicitly deferred. Configure adapter-node with `precompress: false`, so the production contract and handler agree.

### Production build and embed contract

`example/web/vite.config.ts` is the only Kit configuration file; no `svelte.config.js` is created. It uses:

```ts
sveltekit({
  adapter: adapter({ out: 'build', precompress: false }),
  paths: {
    origin: process.env.ORIGIN ?? 'http://127.0.0.1:8080',
    relative: false,
  },
})
```

`example/web/src/routes/+layout.ts` exports `ssr = false` and `prerender = true`. This makes Kit write `build/prerendered/index.html`, a route-independent CSR boot document with absolute asset paths. `example/web/src/routes/items/[id]/+page.ts` exports `prerender = false`, because a required dynamic parameter cannot be enumerated at build time.

`example/web/assets.go` is an ordinary Go package (`package web`) colocated above the generated output:

```go
//go:embed build/client build/prerendered
var Build embed.FS
```

The application derives `fs.Sub(web.Build, "build/client")` and `fs.Sub(web.Build, "build/prerendered")`, then passes those views to `NewStaticHandler`. Although adapter-node also emits `build/index.js`, `build/handler.js`, and server chunks, the embed patterns exclude them.

The build order is intentionally `vp build` before any command compiling `example/web`. This makes a missing frontend artifact a compile-time error. Staleness is addressed operationally in this sprint by always rebuilding before the production acceptance run; content-addressed build manifests belong to a later sprint.

### Development process model

`example/Procfile` owns two long-running commands:

```procfile
web: cd web && mise x -- vp dev --host 127.0.0.1 --port 5173 --strictPort
go: mise x -- air -c .air.toml
```

`example/.air.toml` builds `./cmd` to `./tmp/skgo-example` and starts it with `--listen 127.0.0.1:8080 --proxy http://127.0.0.1:5173`. It watches `.go` files and excludes `tmp`, `web/build`, `web/node_modules`, `e2e/node_modules`, and Playwright output. Air never launches Vite and Go never launches Node; overmind is the sole supervisor.

The example command has exactly two flags:

```text
--listen string   public listen address (default "127.0.0.1:8080")
--proxy URL       dev target; empty selects embedded production mode
```

`main` chooses the proxy when `--proxy` is non-empty and otherwise constructs the embedded handler. It wraps either mode with `X-Skgo-Mode: dev` or `X-Skgo-Mode: prod`, which gives the browser suite direct evidence that its document response crossed the Go server. It owns `http.Server` and returns startup/configuration errors before calling `ListenAndServe`.

### Browser acceptance shape

`example/e2e` is its own pnpm package. `playwright.config.ts` calls `defineBddConfig({ features: 'features/**/*.feature', steps: 'steps/**/*.ts' })`, uses `BASE_URL`, runs Chromium serially with zero retries, and retains traces on failure. It never starts a server; the dev and prod acceptance commands start the real target first.

One feature contains four scenarios:

1. Home renders the text owned by `Greeting.svelte`, and the initial response has the expected `X-Skgo-Mode`.
2. Clicking the item link renders item `42` while the count of navigation-resource entries remains one.
3. Direct navigation to `/items/42` renders the same dynamic-route content.
4. Direct navigation to `/not-a-route` renders the custom SvelteKit error page.

The same generated Playwright tests run with `EXPECTED_MODE=dev` and `EXPECTED_MODE=prod`.

## Implementation Plan

### Phase 1: Establish reproducible workspace and toolchain

Files:

- `go.mod`
- `go.work`
- `example/go.mod`
- `example/mise.toml`
- `example/web/package.json`
- `example/web/pnpm-lock.yaml`
- `example/web/pnpm-workspace.yaml`
- `example/web/tsconfig.json`
- `example/e2e/package.json`
- `example/e2e/pnpm-lock.yaml`
- `example/e2e/tsconfig.json`
- `.gitignore`

Tasks:

1. Create the root and example modules plus `go.work`; prove `go list ./...` and `go list ./example/...` resolve without downloading Go dependencies.
2. Pin the mise tools in `example/mise.toml`: Go `1.27.1`, Node `24.16.0`, Vite+ `0.3.0`, Air `1.67.4`, and overmind `2.5.1`.
3. Pin the web package exactly: `@sveltejs/kit` `3.0.0-next.25`, `@sveltejs/adapter-node` `6.0.0-next.10`, `svelte` `5.56.10`, `@sveltejs/vite-plugin-svelte` `7.3.0`, `vite` `8.2.2`, `typescript` `6.0.3`, and `@types/node` `26.4.1`. Set pnpm `11.25.0` under `devEngines.packageManager` with `onFail: "download"`.
4. Pin the E2E package exactly: `@playwright/test` `1.62.1`, `playwright-bdd` `9.2.0`, `typescript` `6.0.3`, and `@types/node` `26.4.1`. `playwright-bdd` 9.2.0 supports Node 20+ and peers with Playwright 1.44+, so these pins are compatible.
5. Use Kit 3 conventions: `tsconfig.json` extends `$app/tsconfig`; `package.json` contains both `"#lib": "./src/lib/index.ts"` and `"#lib/*": "./src/lib/*"` imports; do not create `svelte.config.js`.
6. Generate and commit both pnpm lockfiles with the pinned Vite+ install gesture, not npm/npx.

Commands:

```sh
cd example
mise install
cd web
mise x -- vp install
cd ../e2e
mise x -- vp install
mise x -- node node_modules/@playwright/test/cli.js install chromium
```

After the first lockfile generation, all later installs use `mise x -- vp install --frozen-lockfile`.

### Phase 2: Implement the development proxy test-first

Files:

- `proxy.go`
- `proxy_test.go`

Tasks:

1. Write table/unit tests proving method, path, escaped path, query, body, inbound Host, and forwarding headers reach an `httptest.Server` unchanged where required.
2. Write invalid-target tests for nil, relative, non-HTTP, query-bearing, and fragment-bearing target URLs.
3. Write a raw TCP upgrade test: an upstream handler hijacks a request with `Connection: Upgrade` and `Upgrade: websocket`, returns `101 Switching Protocols`, echoes bytes, and the client connects through the Go proxy. This exercises the standard-library WebSocket tunnel without adding a Go WebSocket dependency.
4. Implement `NewDevProxy` with `httputil.ReverseProxy.Rewrite`, `ProxyRequest.SetURL`, `ProxyRequest.SetXForwarded`, preserved public Host, and a deterministic 502 error response.
5. Run `go test ./...` and `go vet ./...`.

### Phase 3: Implement the embedded static/SPA handler test-first

Files:

- `static.go`
- `static_test.go`

Tasks:

1. Build tests with `testing/fstest.MapFS` for exact client files, root and nested prerendered candidates, deep-link fallback, unknown HTML fallback, non-HTML 404, missing immutable 404/no-store, MIME, cache headers, ETag/304, HEAD, unsupported methods, directories, encoded traversal, and invalid constructor options.
2. Implement `NewStaticHandler` and private path/file-serving helpers using only `io/fs`, `net/http`, `mime`, and standard hashing/path packages.
3. Keep request dispatch independent of disk paths. The handler accepts `fs.FS`, making `embed.FS` production behavior identical to MapFS unit tests.
4. Do not add brotli/gzip negotiation, range requests beyond what `http.ServeContent` supplies, directory listings, dotfile policy, or a generated MIME table in this sprint.
5. Run `go test ./...` and `go vet ./...`.

### Phase 4: Build the real Kit 3 application and production embed

Files:

- `example/web/vite.config.ts`
- `example/web/tsconfig.json`
- `example/web/src/app.html`
- `example/web/src/lib/index.ts`
- `example/web/src/lib/Greeting.svelte`
- `example/web/src/routes/+layout.ts`
- `example/web/src/routes/+page.svelte`
- `example/web/src/routes/+error.svelte`
- `example/web/src/routes/items/[id]/+page.ts`
- `example/web/src/routes/items/[id]/+page.svelte`
- `example/web/assets.go`
- `example/cmd/main.go`

Tasks:

1. Configure adapter-node with `out: 'build'`, `precompress: false`, `paths.relative: false`, and `paths.origin` from `ORIGIN` defaulting to the Go URL.
2. Implement the root page from a real `Greeting.svelte` component and a Kit link to `/items/42`.
3. Make the root layout CSR-only and prerendered. Make the required-parameter item page non-prerendered and render `params.id` via a universal `+page.ts` load—no server load.
4. Add a distinctive `+error.svelte` that renders the client router's status/message.
5. Run `ORIGIN=http://127.0.0.1:8080 mise x -- vp build` and inspect `build/prerendered/index.html`, `build/client/_app/version.json`, `build/client/_app/manifest.js`, and hashed `build/client/_app/immutable/entry/{start,app}.*.js` before writing the embed package.
6. Add `assets.go` with only `build/client` and `build/prerendered` embed patterns. Confirm `go tool nm`/binary string inspection does not reveal adapter-node's `build/handler.js` banner or server entrypoint text.
7. Implement `main.go` flag parsing, mode selection, `fs.Sub`, handler construction, `X-Skgo-Mode`, and `http.Server` startup.
8. With Vite stopped, run `go build -o /tmp/skgo-example ./example/cmd`, start it, and use `curl` to prove `/`, `/items/42`, `/_app/version.json`, a referenced immutable entry chunk, and a missing immutable chunk return the planned status/header shapes.

If the root prerendered `index.html` does not boot `/items/42` or `/not-a-route`, stop and inspect the generated document against `ephemeral/semantic-index/sources/kit/server-runtime/csr-shell.md`. The bounded fallback is to replace the root-document reuse with Kit's own `builder.generateFallback`; do not hand-author the boot script.

### Phase 5: Wire Air, Vite+, and overmind

Files:

- `example/.air.toml`
- `example/Procfile`

Tasks:

1. Configure Air to build `./cmd` into `./tmp/skgo-example`, pass `--listen 127.0.0.1:8080 --proxy http://127.0.0.1:5173`, and exclude frontend dependencies/builds and E2E output.
2. Add the two-process Procfile shown in Architecture. Use fixed ports and `--strictPort` so a collision fails loudly instead of silently changing the origin.
3. Start `overmind start`; verify the browser's document URL and responses use port 8080, not 5173.
4. Edit visible text in `Greeting.svelte`; verify it changes without a document reload and that the browser's Vite HMR WebSocket is connected through port 8080.
5. Touch a Go source file; verify Air reports a successful rebuild and the serving process PID changes, then reload the page successfully through the restarted Go server.

### Phase 6: Add and run Playwright-BDD acceptance

Files:

- `example/e2e/playwright.config.ts`
- `example/e2e/features/navigation.feature`
- `example/e2e/steps/navigation.ts`
- `example/e2e/.gitignore`

Tasks:

1. Define the four Gherkin scenarios from Browser acceptance shape.
2. Implement steps with `createBdd()` from `playwright-bdd`, Playwright's `page`, response headers, accessible roles/text, and the browser Performance API. Do not use fixed sleeps.
3. Generate native Playwright tests and run them against the already-running dev stack:

```sh
cd example/e2e
BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=dev \
  mise x -- node node_modules/playwright-bdd/dist/cli/index.js
BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=dev \
  mise x -- node node_modules/@playwright/test/cli.js test
```

4. Stop overmind, rebuild production, start only the Go binary, then run the identical suite:

```sh
cd example/web
ORIGIN=http://127.0.0.1:8080 mise x -- vp build
cd ../..
go build -o /tmp/skgo-example ./example/cmd
/tmp/skgo-example --listen 127.0.0.1:8080
```

In another terminal:

```sh
cd example/e2e
BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=prod \
  mise x -- node node_modules/playwright-bdd/dist/cli/index.js
BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=prod \
  mise x -- node node_modules/@playwright/test/cli.js test
```

5. Finish with the ordinary repository gates:

```sh
go test ./...
go test ./example/...
go vet ./...
go vet ./example/...
```

## Files Summary

| File or directory | Change | Purpose |
|---|---|---|
| `go.mod` | Create | Root `github.com/tylergannon/skgo` library module, Go 1.27.1. |
| `go.work` | Create | Make root and `example` modules addressable from root commands. |
| `proxy.go`, `proxy_test.go` | Create | Public Vite+ reverse proxy and HTTP/WebSocket contract tests. |
| `static.go`, `static_test.go` | Create | Public `fs.FS` production handler and routing/cache tests. |
| `example/go.mod` | Create | Runnable example module requiring the local skgo library. |
| `example/mise.toml` | Create | Exact Go, Node, Vite+, Air, and overmind tools. |
| `example/Procfile` | Create | Supervise `vp dev` and Air together. |
| `example/.air.toml` | Create | Rebuild/restart the Go command in proxy mode. |
| `example/cmd/main.go` | Create | Flags, handler selection, mode header, and socket ownership. |
| `example/web/package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml` | Create | Exact Kit 3 frontend dependency boundary. |
| `example/web/vite.config.ts`, `tsconfig.json` | Create | Flat Kit 3 config, adapter-node output, fixed origin, absolute paths. |
| `example/web/src/app.html` | Create | Standard Kit document template. |
| `example/web/src/lib/index.ts`, `Greeting.svelte` | Create | Export and render a real reusable Svelte component. |
| `example/web/src/routes/+layout.ts` | Create | Declare `ssr = false`, `prerender = true`. |
| `example/web/src/routes/+page.svelte` | Create | Home route and client-navigation link. |
| `example/web/src/routes/+error.svelte` | Create | Observable Kit client-router error page. |
| `example/web/src/routes/items/[id]/+page.ts` | Create | Universal param load and `prerender = false`. |
| `example/web/src/routes/items/[id]/+page.svelte` | Create | Dynamic route UI. |
| `example/web/assets.go` | Create | Embed adapter-node's client/prerendered trees only. |
| `example/e2e/package.json`, `pnpm-lock.yaml`, `tsconfig.json` | Create | Independent pinned Playwright-BDD package. |
| `example/e2e/playwright.config.ts` | Create | BDD generation and external-`BASE_URL` Playwright setup. |
| `example/e2e/features/navigation.feature` | Create | Four user-visible dev/prod scenarios. |
| `example/e2e/steps/navigation.ts` | Create | Browser steps and Go-path evidence. |
| `.gitignore`, `example/e2e/.gitignore` | Create | Ignore generated builds, Air temp files, dependencies, generated BDD tests, and Playwright artifacts. |

No documentation, sprint ledger, code generator, custom adapter, remote-function stub, server load, or CI workflow is added in this sprint.

## Definition of Done

- [ ] The exact toolchain installs from the committed lockfiles with `mise install` and `mise x -- vp install --frozen-lockfile` in both JS packages.
- [ ] `go build ./example/cmd` works from the repository root after the frontend build because `go.work` includes both modules.
- [ ] `NewDevProxy` and `NewStaticHandler` expose the exact APIs in this plan and have no non-stdlib Go dependencies.
- [ ] Go unit tests demonstrate request/Host/query/body forwarding, forwarding headers, a real 101 upgrade tunnel, static/prerendered lookup, deep-route document fallback, immutable-miss isolation, MIME/cache behavior, ETag/304, HEAD, 405, and traversal rejection.
- [ ] `overmind start` launches exactly Vite+ and Air; the browser connects only to Go at port 8080.
- [ ] A `.svelte` edit visibly hot-updates through the Go-port WebSocket without a document reload.
- [ ] A `.go` edit causes Air to build successfully, replaces the serving PID, and the browser succeeds after restart.
- [ ] `build/prerendered/index.html` is generated by Kit with absolute `/_app/...` boot imports; Go does not synthesize it.
- [ ] With Vite stopped, the standalone binary serves `/`, direct `/items/42`, client navigation, and Kit's custom unknown-route error UI.
- [ ] The embedded binary contains `build/client` and `build/prerendered`, and inspection confirms adapter-node's generated Node handler/server entry is absent.
- [ ] The four Playwright-BDD scenarios pass unmodified against both `EXPECTED_MODE=dev` and `EXPECTED_MODE=prod`; every initial document response reports the expected Go mode.
- [ ] `go test ./...`, `go test ./example/...`, `go vet ./...`, and `go vet ./example/...` all pass after the final frontend build.
- [ ] No remote functions, server loads, code generation, SSR, project generator, custom adapter, shell test runner, evidence manifest, or proof framework appears in the diff.

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| A root route prerendered under `ssr = false` may not behave exactly like Kit's generated SPA fallback on a deep link. | Make deep-link and unknown-route browser scenarios early Phase 4 gates. If either fails, switch to Kit's `builder.generateFallback` path; never hand-write the boot document. |
| `go:embed` makes all example Go commands depend on a prior frontend build. | Treat this as the explicit stale/missing-build policy for Sprint 001; document and exercise the exact `vp build` prerequisite. A later build-orchestration sprint may improve the gesture. |
| Vite HMR uses a WebSocket path outside pinned Kit source and Vite+ can change it. | Proxy all upgrades and all paths without interception; unit-test a generic upgrade tunnel and live-test Vite+ 0.3.0 through port 8080. |
| Preserving the browser Host can trigger Vite's allowed-host check. | Use loopback hosts in this sprint and fixed `127.0.0.1` URLs. If Vite rejects the public host, add that exact host to `server.allowedHosts`; do not rewrite the public origin silently. |
| Adapter-node emits much more than Go needs. | Embed patterns name only `build/client` and `build/prerendered`; inspect the final executable for known Node handler text. |
| Missing immutable chunks could accidentally receive HTML and create confusing module-parse errors. | Give the immutable prefix a hard 404/no-store branch before document fallback and lock it with a unit test. |
| Browser 404 UI is rendered after an HTTP 200 document fallback. | Make the sprint claim about visible Kit error behavior, not an HTTP 404 status. Route-aware status codes require a manifest and belong to a later sprint. |
| Live npm/tool releases could drift from the researched stack. | Commit exact versions and lockfiles. Do not use ranges or refresh pins during implementation unless a pinned install is reproducibly broken. |
| Playwright-BDD generation can hide what actually ran. | Run `node .../playwright-bdd/dist/cli/index.js` explicitly, then the pinned Playwright CLI directly; configure zero retries and retain traces on failure. |
| Static behavior can balloon toward full adapter-node/sirv parity. | Hold Sprint 001 to exact files, HTML fallback, immutable caching, MIME, ETag/304, and method/path safety. Precompression, full mrmime, redirects, ranges, bases, and manifests are later work. |

## Dependencies

### Go and process tools

| Dependency | Exact version | Use |
|---|---:|---|
| Go | `1.27.1` | Library, example command, embed, tests. |
| Air (`air-verse/air`) | `1.67.4` | Dev rebuild/restart. |
| overmind | `2.5.1` | Run Vite+ and Air from the Procfile. |
| mise | existing bootstrap tool | Install the exact tool versions above; do not encode a mise self-version in the project. |

### Frontend build package (`example/web`)

| Package/tool | Exact version |
|---|---:|
| Node | `24.16.0` |
| Vite+ (`viteplus`) | `0.3.0` |
| pnpm | `11.25.0` |
| `@sveltejs/kit` | `3.0.0-next.25` |
| `@sveltejs/adapter-node` | `6.0.0-next.10` |
| `svelte` | `5.56.10` |
| `@sveltejs/vite-plugin-svelte` | `7.3.0` |
| `vite` | `8.2.2` |
| `typescript` | `6.0.3` |
| `@types/node` | `26.4.1` |

### Browser acceptance package (`example/e2e`)

| Package | Exact version |
|---|---:|
| `@playwright/test` | `1.62.1` |
| `playwright-bdd` | `9.2.0` |
| `typescript` | `6.0.3` |
| `@types/node` | `26.4.1` |

The implementation also requires Chromium installed by the pinned Playwright CLI. Production runtime dependencies are only the compiled Go binary and operating-system networking; Node, Vite+, Air, and overmind are development/build tools.

## Open Questions

No product decision blocks implementation. These are bounded questions that the sprint must answer by running the real build:

1. Does adapter-node 6.0.0-next.10 emit the root `ssr = false`, `prerender = true` document in a form that boots both `/items/42` and `/not-a-route` when reused as the production document fallback? The browser scenarios decide; Kit's own `generateFallback` is the only permitted fallback approach.
2. Does Vite+ 0.3.0 require an explicit `server.allowedHosts` entry when the upstream request preserves `Host: 127.0.0.1:8080`? Start without it and add only the exact loopback entry if live dev proves it necessary.
3. Does `playwright-bdd` 9.2.0's generator accept the researched `defineBddConfig` layout unchanged with Playwright 1.62.1? Resolve during Phase 1 before writing feature steps; retain both exact pins unless the package reports a real incompatibility.
4. Which adapter-node-generated string is stable enough to prove Node server code was not embedded? Choose one observed from `build/handler.js` after the first build, then pair a negative binary string check with positive checks for the Svelte component text and an immutable chunk name.

