# Sprint 001 — Orchestrator critique of all drafts

Every claim below was checked against the pinned source under
`ephemeral/inspiration/` or the installed toolchain, not against the other drafts.
Cite: `reference/kit/packages/...` paths are relative to the token cache root.

## Facts settled by source (apply to every draft)

1. **Prerendered home page path.** `builder.writePrerendered(dest)` copies
   `output/prerendered/pages`, `/dependencies`, `/data` *flattened* into `dest`
   (`reference/kit/packages/kit/src/core/adapt/builder.js:L223-L231`), and `/`
   prerenders as `index.html` (`core/postbuild/prerender.js:L248-L253`). So the file is
   `example/web/build/prerendered/index.html`. **Gemini correct; Claude wrong**
   (`prerendered/pages/index.html`).
2. **adapter-node wipes `out` on every build** (`reference/kit/packages/adapter-node/index.js:L23`
   `rmSync(out, {force:true, recursive:true})`). Any checked-in placeholder under
   `build/` is deleted by every `vp build` and reappears as a git deletion. **Gemini's
   `.gitkeep` guard does not survive a build.** Either accept "fresh checkout cannot
   `go build` until `vp build` runs" (Codex's stated position, and the junkyard's
   "stale frontend fails loudly" principle) or embed from a directory adapter-node
   does not own.
3. **HMR through the proxy needs no Vite config.** Vite 8.2.2 substitutes
   `__HMR_PORT__ = hmr.clientPort || hmr.port || null`; the client then uses
   `importMetaUrl.port`, i.e. the port the page was served from
   (mise-installed vite 8.2.2 `dist/node/chunks/node.js` around L25470-L25500,
   `dist/client/client.mjs:L863-L864`). So the browser opens `ws://<go-host>:<go-port>/`
   and Go's `httputil.ReverseProxy` tunnels it. **Both drafts' HMR claims are
   right, but for the wrong reason (they cite "general Vite knowledge").**
   Trap for the DoD: Vite also bakes `__HMR_DIRECT_TARGET__` (`127.0.0.1:5173`) and
   falls back to a *direct* socket if the proxied one fails, so "HMR works" does not
   prove "HMR works through Go". A dev-mode check must observe the upgrade at the Go
   port (e.g. proxy log line, or a Go test with a fake HMR endpoint), not just a
   hot-updating browser.
4. **`//go:embed` needs `all:`** for `_app/` (underscore-prefixed dirs are excluded
   without it). Both drafts use `all:`; Claude adds a unit test asserting a known
   `_app/immutable/` path is present. Keep that test.
5. **`embed.FS` ModTime is zero**, so sirv's `W/"<size>-<mtimeMs>"` ETag would be
   constant per size. Claude's content-hash ETag is the right deviation. Gemini's
   plan copies the sirv formula verbatim — it would emit colliding ETags.
6. **Vite host check.** Vite 6+ `server.allowedHosts` validates the `Host` header.
   Gemini rewrites `Host` to the target (safe). Claude forwards the browser's `Host`
   unchanged so kit sees the user-facing origin; with `--listen 127.0.0.1:4173` that
   Host is `127.0.0.1:4173`, which Vite accepts as a loopback host. Either works
   locally; Claude's is the better default for kit's origin derivation. Note it.
7. **Prerender gate.** With `prerender = true` on the root layout, kit crawls links
   from `/` (`prerender.js:L473-L518`). Gemini's layout links to `/items/42` and
   `/non-existent`; crawling `/items/42` hits a `prerender = false` page (skipped,
   fine) but `/non-existent` would be a 404 during prerender, and kit's default
   `prerender.handleHttpError = 'fail'` **fails the build**. Remove the
   `/non-existent` link from the layout or set `handleHttpError: 'warn'`.

## Claude draft

Strengths worth keeping:
- Route-aware 200/404 for the boot document (hand matcher for `/` and `/items/{id}`),
  giving real HTTP 404s for unknown paths while still booting kit's error page. This
  is what kit's own server does and what the junkyard's 404 tests assert.
- Content-hash ETags; the `all:` embed unit test; generated mrmime table from
  `reference/mrmime/deno/mod.ts` with the source commit recorded.
- Explicit "no colocation link tree this sprint" and "no CSRF this sprint" scoping.
- Origin reasoning: dev mode needs no `ORIGIN`; prod build must set it to the
  prod listen address.
- Names the playwright-bdd API as unverified prior art and says to check the
  installed package first.

Weaknesses:
- Wrong prerendered path (`prerendered/pages/index.html`); see fact 1.
- **The Go-driven proof harness (`example/e2e_test.go` under `-tags e2e` shelling
  out to `vp build`, `go build`, the binary, `vp dev`, playwright CLI) is proof
  machinery.** The brief says proof is Playwright; AGENTS.md says no harnesses.
  A `go test` that spawns Node, builds the frontend, and parses Playwright's JSON
  is the junkyard's `check-level.sh` in Go clothing. Replace with: a documented
  command sequence (mise tasks or a Procfile entry) and Playwright's own
  `webServer` for prod mode. Keep `go test ./...` for `proxy` and `static` only.
- `.air.toml` `full_bin` with `--listen :4173`: Air's `full_bin` is fine, but
  the draft also sets `bin`; harmless duplication.
- `Procfile` runs `mise x -- air`, which requires Air in `mise.toml` via `aqua:`;
  Air is already installed system-wide here. Either is fine; don't block on mise.
- Pins `@playwright/test` 1.62.1 "for continuity" — there is no ported test code
  yet; use current 1.63.0.
- Port 4173 for Go is a junkyard convention; irrelevant now. Pick one port and
  make `paths.origin` match it (any value works; consistency is what matters).
- "Add a `data-testid=started` marker set in a root `$effect`" — kit already sets
  `body.started`? No: that is the kit test app's own convention
  (`sources/kit/test-apps/harness-helpers.md`), not kit runtime behavior. The marker
  is needed; the draft is right to add one.

## Gemini draft

Strengths worth keeping:
- Correct prerendered path; complete, concrete `package.json`, `vite.config.ts`,
  `tsconfig.json`, four Gherkin features with step definitions, Procfile, Air config.
- Precompression negotiation and immutable caching included (cheap, and adapter-node
  builds `.br/.gz` by default).
- Reasoned open question on 200-vs-404 for unknown routes, recommending 200 for
  this sprint (acceptable), with route-aware 404 deferred.

Weaknesses:
- `.gitkeep` placeholder guard is deleted by every `vp build` (fact 2).
- sirv ETag formula on `embed.FS` (fact 5).
- Layout links to `/non-existent` will fail the prerender crawl (fact 7).
- `.air.toml` with `root = ".."` and `cd example && go build` is contorted; run Air
  from `example/` with `root = "."` and exclude `web`, `e2e`, `tmp`. Air also watches
  `../` root library files only if `root` includes them: with `root="."` in
  `example/`, edits to the root `skgo` package will not trigger a rebuild. The draft
  chose `root=".."` for that reason; better: `root = ".."` with `include_dir`
  limited, or accept Go-library edits need a manual restart this sprint. State it.
- Go version `1.24.0` in `go.mod` when the machine has 1.27.1 and the junkyard used
  1.27; use `go 1.27`.
- `pnpm-workspace.yaml` content is wrong: the junkyard's file used
  `minimumReleaseAgeExclude` to admit a fresh `@types/node`, not
  `onlyBuiltDependencies: []`. Copy the junkyard's file (`sources/junkyard/app/toolchain.md`).
- `mise x -- pnpm test` in `example/e2e`: pnpm is provisioned by `devEngines` only
  after `vp install` in that package; the safer launch line is the junkyard's
  `mise x -- node node_modules/@playwright/test/cli.js test` plus `bddgen` via
  `node node_modules/playwright-bdd/dist/cli/index.js`. Verify against the installed
  package.
- Unit test list says missing immutable assets return `Cache-Control: no-store`;
  adapter-node returns kit's `/_app/*` 404 (`public, max-age=0, must-revalidate`) —
  minor, pick one and cite it.
- `Then 0 document reloads should have occurred` uses a module-level counter shared
  across scenarios; fine with `workers: 1`, fragile otherwise. Use a fixture.

## Codex draft

Strengths worth keeping:
- **Clean-checkout policy stated honestly**: no placeholders; `go build` of the example
  fails at the embed directive until `vp build` has run. This is consistent with
  fact 2 and with the junkyard's "stale frontend fails loudly".
- **`X-Skgo-Mode: dev|prod` response header** set by `main.go` and asserted by the
  Playwright steps. Cheapest possible evidence that the document crossed Go and not
  Vite directly. Adopt.
- Real reusable component (`src/lib/Greeting.svelte` imported via `#lib`) so the
  scenario proves a Svelte component, not just a route file.
- Uses `httputil.ReverseProxy.Rewrite` + `ProxyRequest.SetURL/SetXForwarded` (the
  current Go API, not the deprecated `Director`), 502 `ErrorHandler`, and a raw-TCP
  upgrade test with a hijacking upstream — no websocket dependency.
- Tests with `testing/fstest.MapFS` so `embed.FS` and unit tests exercise the same
  handler. Explicit scope fence: `precompress: false` so build and handler agree.
- Explicit dev checks: browser document URL on 8080, HMR socket connected on 8080,
  Air PID change after a Go edit.
- Commands are the junkyard-validated `mise x -- node node_modules/<pkg>/…cli.js`
  form, never `npx`/`pnpm exec`.

Weaknesses:
- **Critical: `//go:embed build/client build/prerendered` without `all:` silently
  excludes `_app/`** (Go embed skips names starting with `.` or `_` unless the pattern
  has the `all:` prefix). Every hashed chunk would be missing; the binary would serve
  the document and 404 every module. Claude's DoD unit test (assert a known
  `_app/immutable/…` path exists in the FS) is the guard.
- `go.work` at the repo root: fine for local builds, but `replace` already makes
  `example/` build standalone; keeping both is harmless. Note that `go.work` changes
  `go test ./...` semantics at the root (it will include `example/`), which is what the
  intent's success criterion wants anyway.
- Missing-immutable 404 uses `Cache-Control: no-store`; kit's own `/_app/*` 404 uses
  `public, max-age=0, must-revalidate` (`request-pipeline.md` step 12). Minor; pick kit's.
- Stdlib `mime` for everything except `.js/.html`: on macOS `mime.TypeByExtension`
  is env-dependent (`mrmime.md`). Acceptable this sprint since only `.js`, `.css`,
  `.html`, `.json`, `.svg` matter; list `.css`, `.json`, `.svg`, `.webmanifest`, `.map`
  explicitly too, then the stdlib fallback never matters for kit output.
- Unknown route returns 200 + document (like Gemini). Claude's route-aware 404 costs a
  10-line matcher for two patterns; take it, because the junkyard's 404 tests
  (`sources/junkyard/app/e2e-suite.md`) assert the status and we will port them next.
- Pins `@playwright/test` 1.62.1 with no justification; use 1.63.0.
- `example/mise.toml` pins Go 1.27.1 alongside Node/vite-plus, while `example/web`
  needs its own `mise.toml` for `vp` to be on PATH inside that dir (mise config is
  per-directory, inherited from parents — one file at `example/` suffices, and that is
  what Codex does; Claude/Gemini split it across root and `web/`). Either works; say
  which and stick to it.

## Verdict for the merge

Base the final document on **Codex's structure and policies** (honest embed policy,
`Rewrite` API, MapFS tests, `X-Skgo-Mode`, explicit dev-mode checks, command sequences),
take from **Claude** the `all:` embed + guard test, content-hash ETags, route-aware
200/404 matcher, and the "no Go harness / Playwright is run by commands" rule (note
Claude's own draft violates the latter — drop its `e2e_test.go`), and take from
**Gemini** the complete file bodies (`package.json`, `vite.config.ts`, `tsconfig`,
features, steps, Procfile, `.air.toml`) after fixing the `.gitkeep`, ETag, prerender-link
and `pnpm-workspace.yaml` issues.

## Cross-draft consensus I expect the merge to adopt

- Embed from `example/web/dist.go` with `all:build/client all:build/prerendered`;
  a fresh checkout requires `vp build` before `go build` — documented, not hidden.
- Boot document = `build/prerendered/index.html`, served for every non-asset path.
  Status: 200 for known page patterns, 404 otherwise (Claude), using a tiny
  hand-written matcher this sprint.
- Immutable caching for `/_app/immutable/`, content-hash ETags, `.br/.gz`
  negotiation only if it costs under an hour; otherwise next sprint.
- HMR proof: observe the WebSocket upgrade at Go, not just a hot-updating page.
- No Go-driven test harness; Playwright is run by documented commands and
  `webServer` config. `go test ./...` stays Node-free.
- Prerender crawl: the layout must not link to routes that 404.
