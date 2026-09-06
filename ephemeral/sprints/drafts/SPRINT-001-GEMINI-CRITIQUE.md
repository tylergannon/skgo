# Sprint 001 Critique: Claude Draft vs. Codex Draft

**Reviewer:** Gemini (Reviewing `SPRINT-001-CLAUDE-DRAFT.md` and `SPRINT-001-CODEX-DRAFT.md`)  
**Context:** `ephemeral/sprints/drafts/SPRINT-001-INTENT.md`, `ephemeral/brief/2026-09-05-skgo-vision.md`, `ephemeral/plans/2026-09-05-skgo-build-plan.md`  
**Authoritative Sources:** `ephemeral/inspiration/` checked via `ephemeral/semantic-index/README.md` (Route A)  

---

## 1. Executive Summary & Verdict

Both drafts grasp the fundamental objective of Sprint 001: establishing a minimal, working vertical slice where a Go binary serves a CSR-only SvelteKit 3 app (dev proxy to `vp dev` via Air and overmind; embedded production build via `@sveltejs/adapter-node` client/prerendered outputs) proven by Playwright-BDD.

However, each draft suffers from distinct blind spots:

- **Claude's Draft** demonstrates superior protocol and edge-case insight (notably catching the `all:` Go embed requirement, specifying route-aware 200 vs 404 status codes for the CSR shell, and calculating content-hash ETags), but is weighed down by **over-engineering and proof machinery**: a Go test wrapper (`e2e_test.go`) running subprocesses and parsing JSON reports (violating the "no proof machinery" rule), an unnecessary custom MIME generator parsed from `mrmime`, and an incorrect path for prerendered pages (`build/prerendered/pages/index.html`).
- **Codex's Draft** provides a tighter, more pragmatic operational plan for a single agent session (simplifying MIME handling, cleanly disabling adapter precompression with `precompress: false`, adding `go.work` to support root builds, adding `X-Skgo-Mode` response headers, and eschewing proof machinery), but contains a **critical fatal flaw**: it omits the `all:` prefix on `//go:embed build/client build/prerendered`, which causes Go to silently drop the entire `_app/` directory (all JavaScript and CSS chunks). Furthermore, it punts on HTTP 404 status codes for unknown routes (serving HTTP 200 for all HTML fallbacks).

**Verdict:** Neither draft should be accepted wholesale. The optimal sprint plan is a synthesis adopting **Codex's lean phasing, `go.work` module structure, explicit `precompress: false`, and `X-Skgo-Mode` header verification**, combined with **Claude's mandatory `all:` embed prefix with embed unit test, route-aware 200/404 shell matcher, and content-hash ETags**, while firmly rejecting Claude's Go-driven `e2e_test.go` test harness.

---

## 2. Source-Verified Technical Facts (Ground Truth)

Every claim below was audited against the pinned sources under `ephemeral/inspiration/` and the installed toolchain:

### Fact 1: Prerendered Home Page File Path
- In `@sveltejs/kit`, `builder.writePrerendered(dest)` copies `output/prerendered/pages`, `dependencies`, and `data` *flattened* directly into `dest` ([`reference/kit/packages/kit/src/core/adapt/builder.js:L223-L231`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/src/core/adapt/builder.js#L223-L231)).
- `/` prerenders as `index.html` via `output_filename` ([`reference/kit/packages/kit/src/core/postbuild/prerender.js:L248-L256`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/src/core/postbuild/prerender.js#L248-L256)).
- When `@sveltejs/adapter-node` invokes `builder.writePrerendered(`${out}/prerendered${builder.config.paths.base}`)` ([`reference/kit/packages/adapter-node/index.js:L28-L31`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/adapter-node/index.js#L28-L31)), the resulting file lands at:
  `build/prerendered/index.html`
- **Result:** **Codex is correct** (`build/prerendered/index.html`); **Claude is incorrect** (`build/prerendered/pages/index.html`, lines 299, 381, 415).

### Fact 2: `//go:embed` Excludes Underscore Directories Without `all:`
- Go `embed` specification states: *"If a pattern names a directory, all files in the subtree rooted at that directory are embedded (recursively), except that files with names beginning with '.' or '_' are excluded. To include files with names beginning with '.' or '_', use the 'all:' prefix: `all:pattern`."*
- Kit 3 outputs all hashed client chunks, entry scripts, and stylesheets into `<appDir>/immutable/` where `appDir` defaults to `"_app"` ([`reference/kit/packages/kit/src/exports/vite/index.js:L986-L1005`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/src/exports/vite/index.js#L986-L1005); [`types/index.d.ts:L1557-L1563`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/types/index.d.ts#L1557-L1563)).
- Without `all:build/client`, the entire `_app/` directory is silently omitted from the compiled binary.
- **Result:** **Claude is correct and mitigates this explicitly** (lines 45, 287–295, 523–525). **Codex is critically wrong** (line 131: `//go:embed build/client build/prerendered`), producing a broken production binary.

### Fact 3: adapter-node Wipes `out` on Every Build
- `adapter-node` executes `rmSync(out, { force: true, recursive: true })` at the start of `adapt()` ([`reference/kit/packages/adapter-node/index.js:L23`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/adapter-node/index.js#L23)).
- Any checked-in placeholder file (such as `.gitkeep`) inside `example/web/build/` is deleted by every `vp build`.
- **Result:** Codex correctly accepts that a fresh checkout cannot `go build ./example/cmd` until `vp build` runs (Codex lines 53–56, 137–138). Claude's embed structure also places `dist.go` in `example/web/`, requiring a build before compilation.

### Fact 4: `embed.FS` Reports Zero ModTime
- Files in an `embed.FS` report a zero `time.Time{}` for `ModTime()`.
- Sirv computes ETags using `W/"<size>-<mtimeMs>"` ([`reference/sirv/packages/sirv/index.mjs:L116`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/sirv/packages/sirv/index.mjs#L116)).
- A naive sirv ETag port on `embed.FS` generates `W/"<size>-0"`, causing colliding ETags across rebuilds for files of identical byte length.
- **Result:** **Both drafts correctly recognize this** and deviate to content-hash ETags (Claude lines 38, 257; Codex lines 104, 238).

### Fact 5: HMR WebSocket Tunneling and Direct Target Trap
- `httputil.ReverseProxy` in Go ≥ 1.12 handles HTTP/1.1 `Upgrade: websocket` natively.
- Vite generates `__HMR_PORT__ = hmr.clientPort || hmr.port || null`. In the browser, the client connects to `window.location.port` (Go's port) ([`mise` Vite 8.2.2 `dist/client/client.mjs:L863-L864`](file:///Users/tyler/.local/share/mise/installs/npm-vite-plus/0.3.0/node_modules/vite/dist/client/client.mjs#L863-L864)).
- *Trap:* Vite also bakes `__HMR_DIRECT_TARGET__` (`127.0.0.1:5173`) and falls back directly if the proxied connection fails. A visual browser update alone does *not* prove HMR traveled through Go.
- **Result:** Both drafts correctly include WebSocket upgrade unit tests (Claude lines 405–411; Codex line 225), and Codex adds port verification in dev mode (Codex line 286).

### Fact 6: Root Layout Link Crawling During Prerender
- When `prerender = true` is set on root `+layout.ts`, Kit crawls all HTML links found on prerendered pages ([`reference/kit/packages/kit/src/core/postbuild/prerender.js:L473-L518`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/src/core/postbuild/prerender.js#L473-L518)).
- Dynamic route `/items/[id]` with `prerender = false` is safely skipped by the crawler.
- However, linking to an unknown route (e.g. `<a href="/does-not-exist">`) from a prerendered page triggers Kit's default `prerender.handleHttpError = 'fail'` ([`reference/kit/packages/kit/src/core/postbuild/prerender.js:L739-L750`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/src/core/postbuild/prerender.js#L739-L750)), which **aborts the build with an error**.
- **Result:** Both Claude and Codex correctly avoid placing 404 links into `+page.svelte`, testing unknown routes via direct URL navigation.

### Fact 7: Multi-Module Root Compilation (`go.work`)
- The intent calls for: `go build ./example/cmd` executed from the repository root ([`SPRINT-001-INTENT.md:L20, L114`](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-INTENT.md#L20)).
- Because `skgo/` and `example/` are distinct Go modules (`github.com/tylergannon/skgo` and `github.com/tylergannon/skgo/example`), running `go build ./example/cmd` from the root fails under standard Go module mode without `go.work` or `cd example`.
- **Result:** **Codex correctly introduces `go.work`** (Codex lines 61–65, 375). Claude omits `go.work`, requiring `cd example` for builds.

---

## 3. Detailed Evaluation: Claude Draft (`SPRINT-001-CLAUDE-DRAFT.md`)

### Architectural Soundness
- **Strengths:**
  - **Route-aware HTTP status codes for CSR shell:** Claude specifies a `RouteMatcher` in `static.NewHandler` (lines 223–228, 250–253). For known routes (`/`, `/items/{id}`), it serves the shell with HTTP 200; for unknown routes (`/does-not-exist`), it serves the identical shell bytes with HTTP 404. This mirrors Kit's own server runtime contract ([`sources/kit/server-runtime/csr-shell.md:L129-L136`](file:///Users/tyler/src/skgo/ephemeral/semantic-index/sources/kit/server-runtime/csr-shell.md#L129-L136)), allowing Kit's client router to boot while maintaining HTTP protocol correctness.
  - **Embedded `_app/` protection:** Identifies the `//go:embed` underscore exclusion and mandates `all:build/client all:build/prerendered` (lines 287–295).
  - **Content-hash ETags:** Correctly avoids sirv's `mtime`-based formula because `embed.FS` ModTime is zero (lines 257–259).
  - **Host header preservation:** Forwards the inbound `Host` header unchanged to allow Kit to resolve `event.url.origin` correctly without requiring runtime `ORIGIN` configuration in dev mode (lines 208–212).
- **Weaknesses:**
  - **Incorrect prerender path:** Specifies `build/prerendered/pages/index.html` (lines 299, 381, 415). As established in Fact 1, `builder.writePrerendered` flattens the output directory, placing the file at `build/prerendered/index.html`. Using Claude's path will cause an immediate `os.ErrNotExist` at startup.
  - **Proof machinery in `e2e_test.go`:** Wraps frontend building, Go compilation, process management, port polling, and Playwright execution inside a `//go:build e2e` Go test file (lines 452–474). This is the exact pattern forbidden by `AGENTS.md` ("no ledgers/proof machinery") and the vision brief.
  - **Scope creep on MIME table:** Step 2 (lines 385–390) introduces `static/gen/main.go`, which reads `reference/mrmime/deno/mod.ts` and generates a 438-entry committed file `static/mimetable_gen.go`. For Sprint 001, where the application serves only `.html`, `.js`, `.css`, and `.svg`, generating the entire mrmime table is unnecessary overhead.

### Completeness
- Claude provides an extensive architectural rationale and correctly accounts for `devEngines` blocking `npm`/`npx` (line 541).
- However, it sketches Gherkin feature text and configuration files rather than providing complete file contents, leaving implementation details to the executing agent.

### Phasing / Ordering
- Arranged into 8 steps.
- **Flaw:** Step 2 invests heavy effort into MIME generator scripts and sirv testdata table tests before the basic proxy or example app exists. The server fronting Kit routes should be established before optimizing MIME tables.

### Risk Coverage
- Strongest risk table of both drafts (lines 532–544). Covers `//go:embed` directory dropping, `embed.FS` ModTime, modulepreload differences on deep links, `paths.origin` mismatches, and `mise.toml` layering.
- Open questions (lines 585–616) explicitly highlight the modulepreload waterfall gap and the need to verify `playwright-bdd` 9.2.0 APIs against the installed package.

### Feasibility Within One Focused Agent Session
- **Low to Moderate.** The combination of hand-rolling a MIME generator, porting sirv test vectors, and debugging a complex Go subprocess orchestration harness (`e2e_test.go`) significantly increases the probability of session exhaustion before browser tests pass.

### Definition of Done
- 6 clear criteria (lines 504–530).
- **Strong element:** DoD #5 explicitly requires a unit test asserting `fs.Stat` succeeds on a known `_app/immutable/...` asset to ensure `all:` is not accidentally removed during refactoring.
- **Weak element:** Relies on `go test -tags e2e` running the proof harness.

---

## 4. Detailed Evaluation: Codex Draft (`SPRINT-001-CODEX-DRAFT.md`)

### Architectural Soundness
- **Strengths:**
  - **Accurate prerendered path:** Correctly identifies that adapter-node writes `build/prerendered/index.html` (lines 99, 126, 381).
  - **`go.work` integration:** Correctly recognizes that `go build ./example/cmd` from the root requires workspace mode ([`go.work`](file:///Users/tyler/src/skgo/go.work)) when `example/` has its own `go.mod` (lines 61–65, 375).
  - **Disabling precompression:** Configures `adapter({ out: 'build', precompress: false })` in `vite.config.ts` (lines 118, 263). This prevents adapter-node from generating `.br` and `.gz` siblings, eliminating the need to write decompression negotiation logic in Sprint 001.
  - **Lightweight MIME handling:** Uses Go standard library `mime` plus explicit overrides for `.js`/`.mjs` (`text/javascript; charset=utf-8`) and `.html` (`text/html; charset=utf-8`) (line 104), avoiding mrmime generator bloat.
  - **`X-Skgo-Mode` header verification:** Adds `X-Skgo-Mode: dev` and `X-Skgo-Mode: prod` response headers (lines 157–158). This allows Playwright-BDD steps to verify conclusively that responses traversed the Go server rather than a direct Vite port.
  - **No proof machinery:** Tests are executed via standard CLI commands (`playwright-bdd` + `@playwright/test`) without a custom Go runner harness (lines 304–330).
- **Weaknesses:**
  - **FATAL FLAW: Omission of `all:` on `//go:embed`:** Line 131 declares:
    ```go
    //go:embed build/client build/prerendered
    var Build embed.FS
    ```
    As proven in Fact 2, without `all:`, Go excludes any directory beginning with `_`. Because Kit places all JavaScript and CSS chunks in `build/client/_app/`, this binary will compile but immediately fail at runtime, returning 404 for every script and stylesheet.
  - **HTTP 200 for 404 Routes:** Lines 101–102 and line 398 state that unknown routes serve the boot document with HTTP 200, leaving error handling purely to Kit's client-side router. While functionally workable in a browser, returning HTTP 200 for non-existent paths is an architectural compromise that breaks HTTP semantics and diverges from Kit's server behavior ([`sources/kit/server-runtime/csr-shell.md:L129-L136`](file:///Users/tyler/src/skgo/ephemeral/semantic-index/sources/kit/server-runtime/csr-shell.md#L129-L136)).
  - **Unnecessary universal load in `+page.ts`:** Task 263 specifies a universal `+page.ts` load for `/items/[id]`. In Svelte 5 runes, dynamic route parameters can be read directly in `+page.svelte` via `$props()` (`let { params } = $props()`) without declaring a `load` function.

### Completeness
- Highly structured across all 6 phases. Specifies exact file locations, package dependencies, and commands.
- Includes full configuration snippets for `go.work`, `main.go`, `assets.go`, `Procfile`, and `.air.toml`.
- Omits full Gherkin step implementations, though scenario titles and verification goals are clearly articulated.

### Phasing / Ordering
- 6 clean phases: Workspace/toolchain → Dev proxy → Static handler → Kit app & embed → Air/overmind wiring → Playwright-BDD acceptance.
- Testing the Go handlers with `fstest.MapFS` in Phases 2 and 3 before writing frontend code ensures Go unit tests pass independently of the Node build state.

### Risk Coverage
- Covers missing frontend builds, loopback Host checking in Vite 6+, immutable asset isolation, and Playwright CLI invocation under `devEngines`.
- **Major omission:** Completely misses the `all:` Go embed trap, rating the embed risk only in terms of stale files.

### Feasibility Within One Focused Agent Session
- **High (contingent on fixing `all:`).** By avoiding custom MIME generators, precompression negotiation, and Go test harness wrappers, Codex's scope fits neatly within a single focused agent session.

### Definition of Done
- 14 granular, checkbox-style criteria (lines 374–387).
- Strong adherence to project rules: requires lockfile reproducibility, standalone binary verification, mode headers, and explicitly forbids proof harnesses.
- Missing an assertion that `_app/**` is present in the embedded filesystem.

---

## 5. Direct Comparative Matrix

| Dimension | Claude Draft | Codex Draft | Source Truth / Recommendation |
|---|---|---|---|
| **Prerendered File Path** | `build/prerendered/pages/index.html` (WRONG) | `build/prerendered/index.html` (CORRECT) | **Codex is correct.** `writePrerendered` flattens output ([`builder.js:L223-L231`](file:///Users/tyler/src/skgo/ephemeral/inspiration/reference/kit/packages/kit/src/core/adapt/builder.js#L223-L231)). |
| **Go Embed Syntax** | `all:build/client all:build/prerendered` (CORRECT) | `build/client build/prerendered` (FATAL FLAW) | **Claude is correct.** `all:` is mandatory to include `_app/`. |
| **Workspace Architecture** | Independent modules + `cd example` | `go.work` linking `.` and `./example` | **Codex is superior.** Enables `go build ./example/cmd` from root. |
| **Unknown Route Status** | Route-aware (404 with shell body) | Route-blind (200 with shell body) | **Claude is superior.** Preserves HTTP status semantics ([`csr-shell.md:L133`](file:///Users/tyler/src/skgo/ephemeral/semantic-index/sources/kit/server-runtime/csr-shell.md#L133)). |
| **Proof Suite Execution** | Go test harness (`e2e_test.go`) spawning Node | Direct CLI commands via `mise x -- node ...` | **Codex is superior.** Avoids forbidden proof machinery. |
| **Go Server Evidence** | Inferred from browser state | `X-Skgo-Mode: dev\|prod` header verification | **Codex is superior.** Explicit wire proof that Go served the request. |
| **Adapter Precompression** | Not configured (defaults to `true`) | Explicitly set to `precompress: false` | **Codex is superior.** Eliminates dead-weight compression files. |
| **MIME Resolution** | Generated 438-entry table from `mrmime` | Stdlib `mime` + `.js`/`.html` overrides | **Codex is superior for Sprint 001.** Prevents scope ballooning. |
| **ETag Calculation** | Content SHA-256 hash | Content-derived hash | **Both correct.** Replaces sirv's broken zero-mtime formula. |
| **Session Feasibility** | At risk due to harness & MIME generator | High (once `all:` embed is patched) | **Codex is leaner.** |

---

## 6. Strongest Ideas Worth Keeping from Each

### From Claude:
1. **Mandatory `all:` embed pattern with unit test:** Use `//go:embed all:build/client all:build/prerendered` in `dist.go`/`assets.go`, accompanied by a unit test asserting `fs.Stat` succeeds on a known `_app/immutable/...` asset path ([Claude lines 287–295, 523–525](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CLAUDE-DRAFT.md#L287-L295)).
2. **Route-aware HTTP 404 status code for CSR shell:** Implement a minimal hand-written `RouteMatcher` in Go for `/` and `/items/{id}`. Known routes receive HTTP 200; unknown routes receive HTTP 404 with the shell body. This preserves proper HTTP status codes while allowing Kit's client-side error page to boot ([Claude lines 223–228, 250–253](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CLAUDE-DRAFT.md#L223-L228)).
3. **Weak ETag & `no-cache` rules:** Explicitly assign `Cache-Control: public,max-age=31536000,immutable` to `/_app/immutable/**` and `no-cache` to `version.json`/`manifest.js` ([Claude lines 241–243](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CLAUDE-DRAFT.md#L241-L243)).

### From Codex:
1. **Root workspace (`go.work`):** Add `go.work` at repo root referencing `.` and `./example`, fulfilling the intent's requirement that `go build ./example/cmd` works seamlessly from the repository root ([Codex lines 61–65](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CODEX-DRAFT.md#L61-L65)).
2. **Explicit `precompress: false`:** Set `adapter({ out: 'build', precompress: false })` in `vite.config.ts`, keeping adapter-node output clean and aligned with the static handler ([Codex line 118](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CODEX-DRAFT.md#L118)).
3. **`X-Skgo-Mode` header:** Emit `X-Skgo-Mode: dev` or `prod` from Go and assert it in Playwright steps, guaranteeing wire proof that requests were proxied or served by Go ([Codex lines 157–158, 165](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CODEX-DRAFT.md#L157-L158)).
4. **No proof machinery:** Remove custom Go subprocess wrappers; run Playwright-BDD via documented CLI commands ([Codex lines 304–330](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CODEX-DRAFT.md#L304-L330)).
5. **Pragmatic stdlib MIME handling:** Avoid compiling custom MIME tables in Sprint 001; override `.js`/`.mjs` and `.html` explicitly on top of Go's `mime` package ([Codex line 104](file:///Users/tyler/src/skgo/ephemeral/sprints/drafts/SPRINT-001-CODEX-DRAFT.md#L104)).

---

## 7. Weaknesses and Gaps to Avoid

1. **Do NOT use `build/prerendered/pages/index.html`:** The file path is `build/prerendered/index.html` ([Fact 1](#fact-1-prerendered-home-page-file-path)).
2. **Do NOT omit `all:` in `//go:embed`:** Omitting `all:` breaks the production build completely ([Fact 2](#fact-2-goembed-excludes-underscore-directories-without-all)).
3. **Do NOT create `example/e2e_test.go`:** Go tests shelling out to Node, building frontends, and parsing Playwright JSON are proof harnesses. Playwright is the proof.
4. **Do NOT build a MIME generator in Sprint 001:** Keep Sprint 001 focused on the Go server and SvelteKit route rendering.
5. **Do NOT crawl unknown routes during prerender:** Ensure prerendered pages do not contain `<a href="/does-not-exist">` links, which cause `vp build` to abort with a prerender crawl error ([Fact 6](#fact-6-root-layout-link-crawling-during-prerender)).

---

## 8. Final Synthesis: Recommended Sprint Plan Structure

The implementing agent should construct the merged plan using the following unified blueprint:

```text
skgo/                                    go.mod (github.com/tylergannon/skgo), go.work
  proxy.go, proxy_test.go                skgo.NewDevProxy (httputil.ReverseProxy, Host forwarding, 101 upgrade)
  static.go, static_test.go              skgo.NewStaticHandler (fs.FS, route-aware 200/404 shell, content-hash ETags)
example/                                 go.mod (github.com/tylergannon/skgo/example, replace to ..)
  cmd/main.go                            Flags (--listen, --proxy), X-Skgo-Mode header injection
  web/                                   Vite+ root
    package.json, vite.config.ts         adapter-node (out: 'build', precompress: false, paths.relative: false)
    dist.go                              //go:embed all:build/client all:build/prerendered
    src/routes/+layout.ts                ssr = false, prerender = true
    src/routes/+page.svelte              Home page linking to /items/42
    src/routes/items/[id]/+page.svelte   Dynamic route (prerender = false via +page.ts)
    src/routes/+error.svelte             Custom error page
  e2e/                                   playwright-bdd package
    features/*.feature                   home, navigation, deep-link, unknown-route
    steps/*.ts                           Checks text, navigation count, and X-Skgo-Mode
  Procfile                               overmind: vp dev + air
  .air.toml                              Air rebuilds example/cmd
```

This synthesis achieves full compliance with `SPRINT-001-INTENT.md`, eliminates all discovered technical errors, avoids forbidden proof harnesses, and guarantees feasibility within one focused agent session.
