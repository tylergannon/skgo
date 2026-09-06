# Fixture harness: capturing Kit's remote IDs and driving the fixture end-to-end

**Purpose.** How the junkyard got the one number nobody can predict — Kit's hashed remote
function ID — out of Vite and into a Go registrar, and how `run-slice.mjs` drove
generate → Kit build → Go binary → real Chromium. Judged for what becomes ordinary Go tests
and Playwright specs in skgo, and what is dropped.

All paths relative to the token cache root `ephemeral/inspiration/`.

## Why a capture step exists

Kit assigns each remote function an ID of the form `<hash>/<exportName>` where the hash is
derived from the module path (e.g. `iyl7i3/getMessage` for
`src/routes/(group)/remotes/[id]/messages.remote.ts`). The Go server has to serve
`/_app/remote/<id>`. The junkyard's rule: **never compute or predict the hash; read it from
Kit's own client transform output** (`junkyard/ephemeral/remote-codegen/fixture/vite/skgo-capture.ts:L6-L9`,
`route-go-colocation/orientation.md:L32-L33`). Renaming a module changes the ID
(`14b1yne/getNote` → `qm4w74/getNote`, `validation/final-01.md:L47-L49`), and the ID is
identical in dev and production (`run-slice.mjs:L595-L605`).

Pipeline (two stages):
1. **Stage A** (`skgo remotes generate`): scan Go, write `.remote.ts` wrappers, write
   `app/.skgo/remotes-source.json` — the inventory of `{output, bindings[{exportName, kind}]}`
   per module (`skgo-capture.ts:L13-L16`, `run-slice.mjs:L133-L135`).
2. **Kit build/dev with the capture plugin** → `app/.skgo/remotes-kit.json`
   (`KitCapture`), then **Stage B** (`skgo remotes stage-b --capture ...`) joins capture
   with inventory on `output#exportName` and emits the registrar
   (`run-slice.mjs:L473-L483`; `orientation.md:L32-L33`). `skgo remotes build` wraps
   both plus `go build` (`run-slice.mjs:L547-L566`).

## `skgo-capture.ts` — the Vite plugin

`fixture/vite/skgo-capture.ts:L1-L175`. Registered after `sveltekit()` in
`vite.config.ts:L17-L26` and declares `enforce: "post"` (`L55`) so its `transform` sees
Kit's already-transformed client code.

### The transform hook (the actual capture)
`skgo-capture.ts:L111-L142`:
- skip SSR transforms (`transformOptions?.ssr`), only `*.remote.ts` ids, strip `?query`;
- module identity = path relative to app root, forward slashes;
- regex over Kit's transformed client code:
  `/\.(query|command|query_live)\(['"]([^'"]+\/([^/'"]+))['"]\)/g` → `{kind, id, exportName}`
  (`L119-L121`). Kit's client transform rewrites each export into
  `.query('<hash>/<name>')`, `.command(...)`, or `.query_live(...)`.
- **Strict reconciliation**: every binding in the inventory must appear with the same
  kind, and no extras — otherwise throw (`L123-L138`). The generated wrapper is the only
  source of exports, so a mismatch is a bug.
- write after every module, atomically (tmp + rename), with `complete` flag and a sha256
  digest over all captured code (`L145-L174`).

### Forcing unimported modules through Kit
A remote module no page imports never gets transformed, so it would have no ID. Two
strategies:
- **Production** (`moduleParsed`, `L64-L73`): on first `moduleParsed` per environment, call
  `this.load({id})` for every inventoried module. The SSR pass registers the remote with
  Kit and emits its server entry (Kit's client transform requires that metadata); the client
  pass emits the address. Nothing becomes an entry, so no discovery artifact ships. Runs
  from `moduleParsed` rather than `buildStart` because the loader is a no-op until the graph
  build is under way.
- **Dev** (`configureServer`, `L78-L110`): on the first HTTP request, start a warm loop that
  calls `server.environments.client.transformRequest("/" + module)` for each inventoried
  module, retrying every 500 ms for up to 60 s because Kit generates its dev runtime lazily.
  Uses the Vite 6+ environments API with a fallback to `server.transformRequest(url, {ssr:false})`.

Proven: `getUnused` (imported by no page) got an ID and was served by Go in both lanes
(`run-slice.mjs:L516-L524`, `L616-L630`).

### Capture JSON shape
`{ schemaVersion, profile, mode, base, appDir, origin, digest, complete, mappings[{module, exportName, kind, id}] }`
(`skgo-capture.ts:L159-L169`).

## `run-slice.mjs` — the end-to-end driver

`proof/run-slice.mjs:L1-L953`, entered via `check-slice.sh:L1-L18` which insists on a fresh
absolute output dir and an installed oracle `node_modules`, then runs under `mise x -- node`.

### Setup
- Build the `skgo` CLI from source (`L67`), record git HEAD/dirty, `go version`, Kit pin
  (`L68-L73`), launch headless Chromium (`L74`).
- `assembleCandidate(root)` (`candidate.mjs:L14-L52`): copy `fixture/backend/{go.mod,go.sum,remotes,cmd}`,
  rewrite the relative `replace` to absolute, copy the oracle's `package.json`/lockfile/
  `tsconfig.json`/`mise.toml`/`src/app.html`, **symlink** `oracle/node_modules` into
  `app/`, copy `fixture/vite`, `vite.config.ts`, and `app-overlay/src`. Each negative case
  gets its own fresh candidate.

### Process management (`harness.mjs`)
- `launch` spawns detached with a process group; `stop` sends SIGTERM to the group, then
  SIGKILL, and records `cleanup: "verified"` (`harness.mjs:L13-L52`, `L92-L113`).
- `ready(handle, port)` polls a TCP connect while checking the child has not exited
  (`L58-L81`); `assertPortFree` before anything starts (`L83-L90`).
- `run` = `spawnSync` returning status/stdout/stderr, `must` throws on nonzero (`L126-L145`).
- `treeHashes(root)` sha256 of every file except `node_modules`/`.svelte-kit`/`build`,
  skipping symlinks, for read-only/unchanged-tree claims (`L150-L172`). `stable()` for
  key-sorted JSON compare (`L185-L197`).

### The browser lane (`browserLane`, `L381-L467`)
- Starts a proxy ("switchboard", from the oracle) on `proxyPort` that routes remote-prefix
  traffic to Go and everything else to Node, writing an `exchanges.jsonl` with
  `selected_backend` and `export_name` per exchange (`L387-L398`).
- Opens `${origin}/remotes/m1`, waits on `data-testid` text via a polling helper
  (`text`, `L372-L379`): `doc-title`, `plan-tier`, `payment-kind`, `messages-text`
  (the colocated query), `notes-text`, `doc-live` (`L409-L416`).
- Clicks Save; asserts the title updates and that exactly **one** remote request happened
  during the command (`command.updates(query)` single-flight) (`L418-L427`).
- Mutates state out-of-band by POSTing directly to Go (`externalCommand`, `L360-L370`) and
  waits for the `query.live` element to reflect it (`L429-L431`).
- Closes the context *before* reading exchanges because the live stream only finalizes on
  close (`L433-L436`); then asserts every exchange was served by Go and that no response or
  page error contains the Node tripwire string `was invoked by the Node backend`
  (`L441-L461`). The tripwire is the generated TS wrapper's body when executed server-side
  in Node — the ancestor of the new brief's "TS stubs THROW if executed in JS".

### Dev lane (`developmentRuntime`, `L487-L538`)
`vp dev --host 127.0.0.1 --port N --strictPort` with `SKGO_MODE=dev SKGO_CAPTURE=... ORIGIN=...`;
warm the capture by loading the page once against Node with `waitUntil: "domcontentloaded"`,
then poll for `capture.complete` (`L497-L510`); stage-b + `go build`; launch Go; run the
browser lane against the proxy.

### Production lane (`coldProduction`, `L542-L607`)
`skgo remotes build --build-cmd "vp build"` produces the capture and the Go binary; launch
`node build/index.js` (adapter-node) and the Go binary; same browser lane; `/healthz`
coexistence; then `identity(dev) === identity(production)` over the full sorted
`module#export#kind#id` tuple set (`L595-L605`).

### Other cases (all generator-level, no browser)
- clean-scan-and-placement, placement-rejections, determinism-and-drift, type-contract
  (`svelte-check` green, then two disposable page edits that must fail, then bare-pointer
  and missing-registration generator diagnostics) — `L139-L350`.
- rename-regeneration `L634-L718`; route-colocation `L730-L803`; skgo-test-wrapper
  `L813-L878`; editor-tooling `L886-L953`. Details in link-tree.md.

## What to keep as ordinary Go tests / Playwright, and what to drop

The new skgo brief: Go generator, Go tests, Playwright as proof; no `.mjs` harnesses, no
shell runners, no evidence directories.

### KEEP as Go tests (`go test`)
- Scan + placement of a `(group)/[id]` colocated package with sibling `.remote.ts` output
  (port of `L139-L142`, `L730-L739`).
- Link resolves to authored dir; `go list ./...` clean and silent about the link;
  `go build ./...`; `go mod tidy` — run the real `go` binary from the test via `exec`
  (`L754-L784`).
- Rejected generate leaves the tree byte-identical (tree-hash before/after, `L179-L186`).
- Byte-identical rerun; `--check` read-only and detects drift in generated TS and
  `jsonschema_gen.go` (`L194-L232`).
- Rename prunes the old `.remote.ts` and old link, writes the new (`L683-L695`).
- Missing registration / bare pointer produce diagnostics naming the binding (`L273-L348`).
- `skgo test` argument insertion: `-C`, `-args`, dedupe, exit status (`reviews/sol-01.md`).
- Kit ID join: given a capture JSON and inventory, the registrar imports the link path
  and binds every export (`L616-L622`).

### KEEP as Playwright specs
- The browser lane's assertions on `data-testid` elements: query renders, enum/union
  values through the Go codec, colocated route query renders, command refresh with exactly
  one request, live update triggered by an out-of-band Go POST (`L407-L431`).
- `dev == production` identity of remote IDs (`L595-L605`) — one spec that reads both
  captures.
- A tripwire: a page that would execute a TS stub server-side must fail loudly
  (`L455-L461`), now trivially "the stub throws".

### KEEP WITH CHANGES
- `skgo-capture.ts` **transform + reconciliation** (`L111-L142`): the only proven way to
  get Kit's IDs. Keep the regex and the strict inventory match. The plugin is JS that Kit
  itself requires, which the skgo rules permit. Re-check the regex against the pinned kit 3
  client transform; `query_live` naming and the `.query('id')` call shape are kit-version
  facts.
- Unimported-module forcing (`L64-L110`): keep only if skgo still wants to register
  modules no page imports. If the new brief says "generate handlers at the paths Kit
  calls", unimported modules have no callers and the whole `moduleParsed`/`warm` machinery
  can be dropped — a big simplification.
- Dev-mode capture: the new brief has no JS runtime in production but `vite dev` still
  exists for the developer. The retry loop (`L81-L102`) is fragile (60 s deadline, 500 ms
  poll); consider having the Go dev server request the module URLs itself, or accept that
  dev capture completes on first page load.
- `assembleCandidate` → a Go `testdata` fixture dir copied to `t.TempDir()`; the
  `node_modules` symlink trick (`candidate.mjs:L43`) is what made svelte-check and Vite
  fast, keep it.
- Process helpers (`harness.mjs:L13-L122`) → `exec.CommandContext` + process-group kill +
  TCP-poll readiness, in a Go test helper.

### DROP
- `check-slice.sh`, `run-slice.mjs` as runners, `summary.json`/evidence dirs,
  `expectedCases` exact-selection assertion, `identity` git/dirty stamping, `treeHashes` as
  proof-of-read-only theatre (use it only inside a Go test).
- The oracle "switchboard" proxy: it existed because Node and Go both served the app. With
  Go owning the socket, there is nothing to switch.
- adapter-node production lane; `SKGO_MODE`/`SKGO_CAPTURE`/`ORIGIN` env plumbing beyond
  what the skgo adapter needs.
- gopls case as automated proof; keep a manual note.

## Gotchas and traps
- **Vite plugin ordering**: capture must run *after* `sveltekit()` and with `enforce: "post"`
  or it sees untransformed source and the regex matches nothing (`vite.config.ts:L18-L25`,
  `skgo-capture.ts:L55`).
- **SSR transforms** carry different code; skip them (`L112`). The id can carry `?v=` query
  strings; strip before matching (`L113`).
- Production `this.load` must be issued from `moduleParsed`, not `buildStart` (`L60-L63`).
- In dev, Kit transforms a remote module only when the browser first asks; the capture is
  incomplete until a page that imports every module has loaded, hence the warmup page load
  and the `complete` flag (`run-slice.mjs:L497-L507`). Waiting only for `load` raced Vite's
  initial reload; `domcontentloaded` + capture-complete polling fixed it
  (`worklog/route-go-colocation.md:L66-L70`).
- The live stream keeps the exchange open; close the browser context before reading
  exchange logs (`run-slice.mjs:L433-L436`).
- `vp` (vite-plus) is the oracle's Vite front; `vp run check` = svelte-check
  (`run-slice.mjs:L237-L239`).
- Kit query payloads are `base64url(devalue.stringify(arg))` in the `?payload=` query
  string; commands POST `{payload, refreshes}` (`run-slice.mjs:L354-L370`). Go tests that
  hit handlers directly need a devalue encoder or fixed test vectors.
- Every negative case must assemble a fresh candidate; mutating the shared one poisons
  later cases (`L144-L150`, `L171-L173`).
- JavaScript `const` declaration order inside the harness bit the last run
  (`worklog:L75-L78`) — one more reason to move to Go.

## Recipes
- To capture Kit's ID for a `.remote.ts` module: `skgo-capture.ts:L111-L142` (regex at L120).
- To force Kit to transform an unimported module in a production build: `L64-L73`; in dev: `L78-L110`.
- To write the capture file atomically with a completeness flag: `L145-L174`.
- To join capture with inventory and build the Go server: `run-slice.mjs:L475-L483`.
- To drive a dev-mode end-to-end check: `run-slice.mjs:L487-L538`.
- To assert "exactly one request during a command refresh": `run-slice.mjs:L418-L427`.
- To assert a live query updates from an out-of-band mutation: `run-slice.mjs:L360-L370`, `L429-L431`.
- To hash a tree for an unchanged-tree assertion: `harness.mjs:L150-L172`.
- To build a fresh candidate from fixture + shared node_modules: `candidate.mjs:L14-L52`.
