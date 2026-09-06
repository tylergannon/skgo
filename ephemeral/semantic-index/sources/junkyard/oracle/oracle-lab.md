# The junkyard remote-function oracle — what the lab is and how it works

**Purpose.** Describes the "protocol laboratory" in `junkyard/oracle/`: a tiny real SvelteKit 3 app whose kit-generated browser client is pointed, unchanged, at either kit's own Node server or a Go backend through a transparent proxy ("switchboard"), so Go's wire behaviour can be diffed against kit's. This leaf covers the mechanism (bootstrap, pinning, ID harvesting, switchboard, lanes, evidence) so skgo can reuse the technique without the surrounding process.

## What it is (OBSERVED from source)

- A SvelteKit app with one route and two remote modules: `document.remote.ts` exporting `getDocument` (query), `saveDocument` (command), `watchDocument` (query.live), and `secondary.remote.ts` exporting a second query also named `getDocument`. State is a JSON file per lane. — `junkyard/oracle/src/lib/document.remote.ts:L1-L37`, `junkyard/oracle/src/lib/secondary.remote.ts:L1-L7`, `junkyard/oracle/src/lib/document-store.ts:L9-L30`.
- Client-only page (`ssr = false`) that renders a query display, a save form, and a separate live display, all with `data-testid`s so Playwright can read them. — `junkyard/oracle/src/routes/+page.ts:L1`, `junkyard/oracle/src/routes/+page.svelte:L27-L47`; mounted-profile variant with the secondary query at `junkyard/oracle/profiles/mounted/+page.svelte:L1-L54`.
- Two backends for the same client: kit's Node server (Vite+ dev or a built adapter-node server) and a Go fixture executable (`go/cmd/remote-oracle`, linking `go/remote`) — the Go side lives outside this segment. — `junkyard/oracle/README.md:L3-L12,L93-L96`.

## Bootstrapping kit at a pin (OBSERVED from source)

- `bootstrap.sh`: `mise trust` → `mise install` (Node 24.16.0, vite-plus 0.3.0) → `vp install --frozen-lockfile` → install Playwright chromium → `node proof/sources.mjs`. — `junkyard/oracle/bootstrap.sh:L1-L8`, `junkyard/oracle/mise.toml:L1-L3`.
- `proof/sources.mjs` shallow-fetches each upstream repo in `source-lock.json` (kit, devalue, svelte, vite-plugin-svelte, valibot) at its exact commit into `../reference/<name>`, refuses dirty checkouts, then **byte-compares every file under `node_modules/<pkg>/src` with the clone's `src`** so that the installed package is provably the pinned source (223 kit files, 10 devalue). Writes `reference/remote-oracle-verification.json`. — `junkyard/oracle/proof/sources.mjs:L8-L64`, `junkyard/ephemeral/remote-oracle/overview.md:L32-L33`.
- `vite` is overridden to `@voidzero-dev/vite-plus-core@0.3.0` in `pnpm-workspace.yaml`; esbuild builds allowed. — `junkyard/oracle/pnpm-workspace.yaml:L1-L4`, `junkyard/oracle/package.json:L26-L27`.
- Trap: run the locked **local** `vp` under mise's Node; a global copy of the same version fails kit's `RunnableDevEnvironment instanceof` guard. — `junkyard/ephemeral/worklog/20260904-remote-oracle-or1.md:L4`. Trap: `vp install` at the repo root creates stray package files — run from `oracle/`. — `…or1.md:L3`.

## Harvesting kit's real IDs (the registry plugin)

- A Vite plugin with `enforce:"post"` intercepts the **non-SSR** transform of the listed `.remote.ts` modules, regex-extracts `.query|command|query_live('<hash>/<name>')` from kit's generated client code, and writes `manifest.json` = `{schemaVersion, profile, mode, base, appDir, origin, digest, mappings:[{module, kind, id, exportName}]}` plus the raw client module bytes. — `junkyard/oracle/vite/protocol-switchboard.ts:L10-L84`.
- The Go backend consumes that manifest (`--ids manifest.json`) so **kit's own transform output is the ID authority**; nothing recomputes the hash. — `junkyard/oracle/proof/run.mjs:L321-L349`, `junkyard/ephemeral/remote-oracle/proposal.md:L31`.
- Config actually passed to kit (`profile/base/appDir/origin`) is passed into the plugin too so the manifest records what kit received, not a recomputation. — `junkyard/oracle/vite.config.ts:L6-L27`, `junkyard/ephemeral/worklog/202609041200-remote-oracle-or2.md:L5`.
- Registry lifetime: aggregate per dev-server/build lifetime, replace a module's entries on each transform (HMR re-transforms), and warm the dev server by loading a page that imports every remote module **before** reading the manifest or starting Go — the dev transform is lazy. — `junkyard/ephemeral/remote-oracle/sprint-02.md:L58-L60,L99-L103`, `junkyard/oracle/proof/run.mjs:L671-L706`.

## The switchboard (transparent proxy)

- One stable browser-facing origin (`127.0.0.1:4370`) proxies to Node (`:4371`) or Go (`:4372`). Requests whose path starts with `${base}/${appDir}/remote/` go to Go in Go lanes; everything else (HTML shell, scripts, dev module sources) goes to Node. **It never rewrites paths, IDs, or bodies.** — `junkyard/oracle/proof/switchboard.mjs:L6-L34,L60-L86`, `junkyard/oracle/proof/run.mjs:L356-L367`.
- Every remote exchange is appended to `exchanges.jsonl` with method, pathname, search, remote id, request headers, base64 request body, response status/headers, and **response chunks with monotonic timestamps** (so SSE frames are preserved with timing). — `switchboard.mjs:L36-L74`.
- The controls it supports: `node-fallthrough` routes one selected Go-mode call to Node to prove the tripwire fires. — `switchboard.mjs:L22-L28`.
- Tripwire: the Node twin of each remote function appends an ownership event and, when `PROTOCOL_BACKEND=go`, writes a marker and throws, so any leak from Go lane to Node is detected. — `junkyard/oracle/src/lib/document-store.ts:L55-L74`. Fixture config is read per-request from `PROTOCOL_CONFIG` JSON so one Node shell serves many lanes without restart (restarting Vite changes client bytes). — `document-store.ts:L4-L7`, `junkyard/ephemeral/worklog/20260904-remote-oracle-or1.md:L5`.

## Lanes, warmup, and evidence (the runner)

- Per profile/mode: build (prod) or start `vp dev`; unscored warmup; then lanes **Node A → Node B → Go**, each with a fresh Playwright browser context, fresh JSON store, fresh Go process, fresh switchboard. — `junkyard/oracle/proof/run.mjs:L155-L235,L276-L375`.
- Cross-lane checks: all lanes must receive byte-identical client scripts (SHA-256 of every script response, ~88–89 in dev, 11 in prod) and identical manifests; decoded envelopes (`semantic`) must be equal to Node A's. — `run.mjs:L196-L212,L389-L402,L906-L935`.
- Case assertions read the DOM by testid, count `fetch/xhr` requests and document navigations to prove single-flight refresh and no-reload live update. — `run.mjs:L455-L516,L533-L601,L1170-L1175`.
- Controls (deliberately broken Go): N01 Node fallback, N02 omit refresh, N03 wrong live value (flushed complete frame), N21 swapped query IDs in the manifest. A control counts only if the *named* assertion fails after healthy prerequisites pass. — `run.mjs:L213-L233,L519-L531,L554-L557,L574-L584,L831-L840,L1161-L1169`.
- Evidence written per run: `summary.json`, `identity.json` (candidate SHA, dirty state, tool versions, hashes of lock/config/runner/plugin/Go tree/build tree/binary, process records with listener PIDs), `playwright-results.json`, `artifact-index.json`; per lane `result.json`, `exchanges.jsonl`, `ownership.jsonl`, `browser-requests.json`, `client-hashes.json`, `browser.png`. — `run.mjs:L87-L131,L603-L622,L1092-L1123`.
- Process hygiene worth copying: refuse an existing output dir and occupied ports; bounded 30 s readiness with `lsof`/`ps` listener-identity check; detached process groups with SIGTERM→SIGKILL; disable Playwright's own SIGINT/SIGTERM handlers so the runner's cleanup owns them. — `run.mjs:L49,L105,L134,L975-L1062`, `junkyard/ephemeral/worklog/20260904-remote-oracle-or1.md:L8-L9`.

## How to run it (OBSERVED from docs)

```sh
sh ephemeral/remote-oracle/check-sprint-02.sh /tmp/new-dir      # bootstrap + OR1 + OR2 matrix
cd oracle && mise x -- node proof/run.mjs --output /tmp/new-dir --inspect mounted/dev/go   # interactive
mise x -- node proof/run.mjs --output /tmp/x --backend /abs/backend --backend-args '["--flag"]'
```
Runner appends `--listen --document --ids --version --control --events --run-id --lane [--secondary-document]` to any backend. Ports 4370–4372 (override `ORACLE_PORT_0..2`). — `junkyard/oracle/README.md:L14-L96`, `junkyard/ephemeral/remote-oracle/check-sprint-02.sh:L1-L25`, `run.mjs:L25-L54,L328-L349`.

## Reusable verdict for skgo

- Reuse the **technique**, not the harness: (1) pin kit and byte-verify `node_modules` against the clone; (2) scrape kit's generated client for IDs; (3) stable-origin proxy that routes only `/remote/` to Go and records raw exchanges with timestamps; (4) Node-first reference lanes, then Go, comparing decoded envelopes; (5) tripwired Node twins. skgo's version should be a Go test binary + a small Playwright driver, not a 1,242-line runner with score counting.
- skgo differs in one essential way: there is no Node sidecar for remote functions in production, so "Go owns the whole remote prefix" (`proposal.md:L38-L39`) is the *only* mode; the switchboard's Node fallback path is irrelevant, but the Node **reference** lane remains the way to get ground truth.
- Still unverified by the lab and needed by skgo: `form`, `prerender`, `query.batch`, SSR data carriers, rich devalue values and custom `transport`, validation/error/redirect envelopes, live heartbeat/teardown, origin guard. — `junkyard/oracle/README.md:L7-L8`, `run.mjs:L74-L82`, `overview.md:L74-L76`.

## Gotchas and traps
- Fresh Vite dependency optimisation makes the first page load slow; bound waits by an outer deadline and retry Playwright `TimeoutError` (the 1000 ms locator timeout once escaped the 5 s bound). — `junkyard/ephemeral/worklog/202609041200-remote-oracle-or2.md:L8`, `junkyard/ephemeral/reviews/or2-fable-round-02.md:L38`.
- Killing a detached Go process group can return `EPERM`; fall back to the recorded child PID. — `…or2.md:L7`.
- Same-origin browser caches/subscriptions contaminate sequential lanes unless each lane gets a new browser context. — `junkyard/ephemeral/worklog/20260904-remote-oracle-design.md:L13`.
- EMFILE when spawning many processes concurrently; spawn sequentially. — `junkyard/ephemeral/worklog/remote-oracle-go.md:L4`.
- Counting "invocation" and "mutation" events as two writes made A02 fail; emit an explicit mutation event. — `…or2.md:L6`.

## Recipes
- Start a new lab: copy `junkyard/oracle/{mise.toml,package.json,pnpm-workspace.yaml,tsconfig.json,vite.config.ts,bootstrap.sh,source-lock.json}` and `proof/sources.mjs`; drop the scoring.
- Add a remote function and get its ID: edit `src/lib/*.remote.ts`, extend the module allow-list at `junkyard/oracle/vite/protocol-switchboard.ts:L23-L29`, read `manifest.json`.
- Record raw wire traffic: `junkyard/oracle/proof/switchboard.mjs:L36-L58`.
- Decode envelopes for comparison: `junkyard/oracle/proof/run.mjs:L906-L939`.
- Interactive inspection and external store edit: `junkyard/oracle/README.md:L49-L83`.
