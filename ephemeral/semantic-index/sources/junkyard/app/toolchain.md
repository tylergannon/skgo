# Junkyard guestbook — toolchain

**Purpose.** The exact, demonstrated-working JS toolchain the junkyard used to build and test a Kit 3 app: pinned package versions, mise-managed Node + Vite+ (`vp`), the Playwright launch recipe, the build-time origin, and the CI workflow. This is the reference environment the Go server must be able to drive (build the client bundle, run the same suite via `BASE_URL`).

## Key concepts

- **Exact pins (all `devDependencies`, no ranges)** — `junkyard/app/package.json:L14-L23`:
  - `@sveltejs/kit` **3.0.0-next.25**
  - `svelte` **5.56.10**
  - `vite` **8.2.2** (lockfile resolves rolldown **1.2.6** underneath, `junkyard/app/pnpm-lock.yaml:L594`)
  - `@sveltejs/vite-plugin-svelte` **7.3.0**
  - `@sveltejs/adapter-node` **6.0.0-next.10** (reference target only)
  - `@playwright/test` **1.62.1**
  - `typescript` **6.0.3** (deliberately not TS 7 / tsgo — the project CLAUDE.md says TS 7 cannot build kit 3; no `rsvelte` present)
  - `@types/node` **26.4.1**
  - runtime dep: `valibot` **1.4.2** (`junkyard/app/package.json:L31-L33`) — schema for `command`/`form` remote functions.
- **`#lib` import alias** is a Node `imports` map in `package.json`, not a Vite alias — `junkyard/app/package.json:L5-L7` (`"#lib/*": "./src/lib/*"`). Every source file uses `#lib/...` (e.g. `junkyard/app/src/hooks.ts:L3`); `$lib` is never used.
- **Scripts** — `junkyard/app/package.json:L8-L13`: `dev: vite dev`, `build: vite build`, `start: node build/index.js`, `test: playwright test`. In practice the CI and target scripts call `vp` (Vite+) instead of `vite`, and Playwright via its CLI file rather than `pnpm test`.
- **pnpm via devEngines** — `junkyard/app/package.json:L24-L30`: `pnpm 11.25.0`, `onFail: download` (Node's `devEngines` auto-downloads pnpm). `pnpm-workspace.yaml` exists only to whitelist `@types/node@26.4.1` from pnpm's `minimumReleaseAge` gate (`junkyard/app/pnpm-workspace.yaml:L1-L2`) — a brand-new package version would otherwise be refused at install.
- **mise** — `junkyard/app/mise.toml:L1-L3`: `node = "24.16.0"`, `"npm:vite-plus" = "0.3.0"`. `vp` is the Vite+ CLI installed as an npm-backed mise tool; `mise x -- vp install --frozen-lockfile` and `mise x -- vp build` are the install/build gestures (`junkyard/.github/workflows/e2e.yml:L25`, `junkyard/ephemeral/projects/skgo/targets/node.sh:L5`).
- **tsconfig** — `junkyard/app/tsconfig.json:L1-L6`: `extends: "$app/tsconfig"` (kit 3 exposes its generated tsconfig via the `$app/tsconfig` specifier; no `.svelte-kit/tsconfig.json` path), `strict: true`.
- **vite.config.ts is the whole kit config (no svelte.config.js)** — `junkyard/app/vite.config.ts:L5-L16`. The `sveltekit({...})` plugin takes `adapter`, `paths.origin`, `experimental.remoteFunctions: true`, and `compilerOptions.experimental.async: true` (needed for top-level `await` in markup, used by `junkyard/app/src/routes/messages/+page.svelte:L21-L30`).
- **Origin is fixed at build time** — `junkyard/app/vite.config.ts:L9-L10`: `paths: { origin: process.env.ORIGIN ?? 'http://127.0.0.1:4173' }`. adapter-node 6 no longer reads `ORIGIN` at runtime; the build must be told the public origin. Every build command therefore threads `ORIGIN=$BASE_URL` (`junkyard/app/playwright.config.ts:L20`, `junkyard/ephemeral/projects/skgo/targets/node.sh:L5`).
- **Playwright config** — `junkyard/app/playwright.config.ts:L1-L26`: `testDir: 'e2e'`, serial (`fullyParallel: false`, `workers: 1`, `retries: 0`), `reporter: list`, `trace: retain-on-failure`, single `chromium` project. `baseURL = BASE_URL ?? http://127.0.0.1:4173`. **If `BASE_URL` is set, `webServer` is `undefined`** (suite targets an externally started server); otherwise it builds and starts adapter-node itself: `` ORIGIN=${baseURL} vp build && PORT=4173 HOST=127.0.0.1 node build/index.js `` (`L17-L24`).
- **Playwright launch under mise** — `mise x -- node node_modules/@playwright/test/cli.js test` (`junkyard/.github/workflows/e2e.yml:L29`, `junkyard/ephemeral/projects/skgo/check-level.sh:L52`). Browser install: `mise x -- node node_modules/@playwright/test/cli.js install --with-deps chromium` (`e2e.yml:L27`). Invoking the CLI file directly avoids depending on a `playwright` bin shim under pnpm.
- **CI workflow** — `junkyard/.github/workflows/e2e.yml:L11-L34`: single job `reference`, `ubuntu-latest`, 15 min timeout, `working-directory: app`, `actions/checkout@v7`, `jdx/mise-action@v4` with `working_directory: app`, install → chromium → test, and uploads `app/test-results` as `playwright-traces` on failure.
- **Target hook pattern** — `junkyard/ephemeral/projects/skgo/targets/node.sh:L1-L32` and `check-level.sh:L34-L44`: a hook defines `build_target`, `start_target` (sets `TARGET_PID`), `stop_target`, optional `GREP`/`GREP_INVERT`; the driver exports `BASE_URL` (default `http://127.0.0.1:4173`), derives the port from it, waits on `curl -sf $BASE_URL/about`, verifies the started pid owns the port via `lsof`, then runs Playwright with `--list --reporter=json` first to get an exact expected count and fails if any test is skipped/flaky/unexpected (`check-level.sh:L57-L80`). `GREP_INVERT='@host|@native'` (`node.sh:L2`) is a no-op today — no spec carries those tags.

## Citations
- `junkyard/app/package.json:L5-L7` (`#lib` imports map), `:L14-L23` (pins), `:L24-L30` (devEngines pnpm)
- `junkyard/app/mise.toml:L1-L3`
- `junkyard/app/vite.config.ts:L7-L15`
- `junkyard/app/playwright.config.ts:L5`, `:L17-L24`
- `junkyard/.github/workflows/e2e.yml:L24-L29`
- `junkyard/ephemeral/projects/skgo/targets/node.sh:L4-L15`, `:L17-L26`
- `junkyard/ephemeral/projects/skgo/check-level.sh:L16-L18`, `:L36-L44`, `:L52-L63`

## Reusable verdicts
- Package pins (kit/svelte/vite/vite-plugin-svelte/playwright/typescript/valibot) — **REUSE AS-IS**. Proven to install and build together; nothing here is Node-server specific except adapter-node.
- `@sveltejs/adapter-node` pin — **DROP** (or keep only for a reference-comparison lane). skgo ships its own adapter; production has no Node.
- `#lib` imports map + `$app/tsconfig` tsconfig — **REUSE AS-IS**. Kit 3 convention; the Go generator's TS stubs will import `#lib/...` the same way.
- `mise.toml` (node 24.16.0 + vite-plus 0.3.0) — **REUSE AS-IS**. `vp dev` is what the Go dev-mode proxy fronts; `vp build` produces the client bundle Go serves.
- `vite.config.ts` — **REUSE WITH CHANGES**: swap `adapter-node` for the skgo adapter; keep `paths.origin` from `ORIGIN`, keep `experimental.remoteFunctions` and `compilerOptions.experimental.async`. Add whatever CSR-only setting the skgo adapter needs (e.g. an SPA fallback / `ssr: false` at the root layout), since the junkyard config is SSR-on.
- `playwright.config.ts` — **REUSE WITH CHANGES**: keep `BASE_URL`-or-local-webServer shape; change the fallback `webServer.command` to build the client and start the Go binary (or drop the fallback and always require `BASE_URL`).
- CI workflow — **REUSE WITH CHANGES**: same mise-action + cli.js invocation; add a `setup-go` step, build the Go server, export `BASE_URL`, run the suite against it. The adapter-node "reference" lane can remain as a control if desired.
- `check-level.sh` + `targets/*.sh` — **DROP as machinery; REUSE the ideas** (exact-count gate, readiness wait on `/about`, pid-owns-port identity check). Project rules forbid shell test runners; express the same guard as a Go test or in `playwright.config.ts` itself.

## Gotchas
- **No `svelte.config.js`.** All kit options live inside `sveltekit({...})` in `vite.config.ts` (`L7-L15`). Anything that "adds svelte.config.js" is wrong for kit 3.
- **`paths.origin` is baked at build time.** Building with the wrong `ORIGIN` produces absolute URLs / `url.href` that will not match `baseURL`, and `platform.spec.ts:L5` asserts `url === ${baseURL}/whoami`. The Go server's dev/prod ports must match the origin the bundle was built with, or the bundle must be rebuilt per target.
- **`vp` not `vite`.** `package.json` scripts say `vite`, but the working invocations use `mise x -- vp ...` (Vite+ 0.3.0). Do not assume a `vite` binary is on PATH under mise.
- **pnpm `minimumReleaseAge`** blocks freshly published versions; whitelist via `pnpm-workspace.yaml` (`L1-L2`) if bumping a pin to something days old.
- **Playwright bin under pnpm** — call `node node_modules/@playwright/test/cli.js` rather than `npx playwright`/`pnpm exec playwright`; this is what CI and the target scripts do.
- **TypeScript 6.0.3, not 7.** Keep it; TS 7 cannot build kit 3 per the project's SvelteKit facts.
- **`experimental.async` compiler flag is required** for `{(await getBanner()).text}` style markup in `messages/+page.svelte`; forgetting it makes the build fail, not the tests.

## Recipes
- To build the client bundle for a given public URL: `ORIGIN=http://127.0.0.1:4173 mise x -- vp build` — start at `junkyard/ephemeral/projects/skgo/targets/node.sh:L5` and `junkyard/app/vite.config.ts:L10`.
- To run the suite against an already-running server: `BASE_URL=http://127.0.0.1:PORT mise x -- node node_modules/@playwright/test/cli.js test` — start at `junkyard/app/playwright.config.ts:L5,L17` and `junkyard/ephemeral/projects/skgo/check-level.sh:L17,L52`.
- To get an exact test count before running (to detect silently skipped tests): `... cli.js test --list --reporter=json` and walk `suites[].specs[].tests` — `junkyard/ephemeral/projects/skgo/check-level.sh:L58-L63`.
- To wire CI: copy `junkyard/.github/workflows/e2e.yml:L20-L34`, insert Go build + `BASE_URL` export before the `test` step.
- To add a new package pin without pnpm refusing it: `junkyard/app/pnpm-workspace.yaml:L1-L2`.
