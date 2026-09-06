# Junkyard remote-codegen: the design that was planned and shipped

Purpose: the architecture the junkyard settled on for turning `*.remote.go` into SvelteKit
`.remote.ts` modules plus one Go registrar — author API, scan rules, admission rules, output
placement, Kit address capture, registrar shape, and what "Stage A / Stage B" meant. It
shipped and passed its own 7/7 gate, so every element below was demonstrated running against
Kit `3.0.0-next.25` (`a593272`), not just designed.

Sibling leaves: `runtime-interface.md` (the Go contract generated code targets),
`typeglue.md` (Go type -> TS pipeline), `lessons-and-traps.md` (what broke).

## Key concepts and decisions

### Product shape (what a developer sees)

- Developer writes ordinary compilable Go packages containing `*.remote.go`; `skgo` emits
  `.remote.ts` into the Svelte app, lets Kit assign the real client wrapper + address, then
  emits one Go registrar the app mounts on its own mux:
  `skgoremotes.RegisterRemotes(mux)` / `NewHandler() (http.Handler, error)`.
  `junkyard/ephemeral/remote-codegen/plan.md:L12-L31`
- First slice = `query`, `command` with one declared requested-query refresh, `query.live`.
  Client-only page (`ssr = false`); invoking a generated remote during SSR trips a generated
  error. Forms, prerender, batch, loads explicitly deferred.
  `plan.md:L33-L37`, `plan.md:L502-L510`
- Product decision recorded in worklog: Go stays in ordinary compilable packages; only the
  generated TS may be route-local. Route-colocated Go and a "mirror tree" were rejected.
  `junkyard/ephemeral/remote-codegen/worklog.md:L3`

### Kit source facts the design was built on (pin 3.0.0-next.25 / a593272)

- Kit's Vite transform computes the remote hash from the root-relative module path and appends
  `hash/export`; dev learns export kinds by importing the server module, production reads
  analysed build metadata, and Kit rejects default/non-remote runtime exports.
  `plan.md:L39-L47` (cites `reference/kit/packages/kit/src/exports/vite/index.js:643-680`,
  `:703-746`, `src/exports/internal/server/remote-functions.js:11-27`)
- Consequences adopted: generated TS has named remote exports only + erased type exports +
  private helpers; **neither Go nor skgo calculates or hashes a Kit ID**; generated imports are
  plain relative paths (`#lib` alias resolution is not a generator feature).
  `plan.md:L49-L58`
- The existing Go runtime seam was deemed correct and generalized rather than replaced:
  manifest maps logical (module, export, kind) -> Kit ID; `NewHandler` validates one-to-one
  binding; handler owns the prefix. `plan.md:L60-L65`

### Author API (the "authored Go contract")

- Three generic constructors are the entire declaration surface; the exported package var IS the
  binding:
  ```go
  var GetDocument  = remote.Query("getDocument", getDocument)
  var SaveDocument = remote.Command("saveDocument", saveDocument,
      remote.CommandOptions{RefreshRequested: []remote.QueryRef{GetDocument.Ref()}})
  var WatchDocument = remote.Live("watchDocument", watchDocument)
  func getDocument(ctx context.Context, in GetDocumentInput) (Document, error)
  func watchDocument(ctx context.Context, in GetDocumentInput, yield func(Document) error) error
  ```
  `plan.md:L78-L96`
- Rules: exactly one-input signature; binding var must be exported (registrar imports it); TS
  export string must be a valid JS identifier, unique per generated module; constructor calls are
  resolved by `go/types` identity, never by source text / import alias. `plan.md:L98-L104`
- Input/result types must be explicit **named** Go types read from the handler signature; the
  author never restates a signature. `plan.md:L114-L122`
- `RefreshRequested` is a static allow-list of query bindings; runtime refreshes only
  client-requested keys that are in the list; handlers never see a refresh collector.
  Live handler yields typed values and honours ctx cancellation; runtime owns SSE framing.
  `plan.md:L106-L112`

### Scan / discovery rules

- Start at an explicit Go module (`--go-root`); find directories containing `*.remote.go` under
  `--remote-root`; derive import paths from `go.mod`; run `go list` / `go/types` on **only those
  packages and their deps**. Deliberately does NOT load `./...` or `main`, so the first clean
  generation succeeds before the generated registrar package exists.
  `plan.md:L69-L76`, `worklog.md:L13`
- Only build-tag-selected files ending `.remote.go` are declarations; remote packages must be
  importable (not `package main`). `plan.md:L74-L76`
- Implementation mirrored go-gen-jsonschema's loader discipline: `packages.Config` with an
  explicit needs mask, build tags, `decorator.Load` (dave/dst keeps comments), marker calls
  resolved by `PkgPath` + name, `file:line` on every diagnostic, plan-then-apply writes with
  ownership headers. `plan.md:L419-L422`, `junkyard/ephemeral/remote-codegen/typeglue-source.md:L193-L215`
- Reviewer nit (never resolved in text): a filesystem walk under `--remote-root` would descend
  into the copied app's `node_modules`/`.svelte-kit`/`build`; prefer `go list`-based discovery
  or explicit excludes. `junkyard/ephemeral/remote-codegen/reviews/fable-plan-03.md:L64-L75`

### Output placement (TS module)

- One directive only: `//skgo:remote output=src/routes/messages/[id]/data.remote.ts` in the
  file's leading comments (exact no-space `//skgo:` form, position-insensitive). Path is POSIX,
  app-relative, must be under `src/`, must end `.remote.ts`; all declarations in one Go file
  share one TS module; duplicate full paths fail. `plan.md:L126-L139`
- Without a directive: (1) a `.remote.go` already under `app/src/lib` gets a sibling
  `.remote.ts`; (2) a source outside the app maps relative to `--remote-root` beneath
  `app/src/lib`. `plan.md:L151-L169`, `worklog.md:L7`
- **Kit server-only guard applied before writing**: a basename matching
  `server_only_module_pattern` (`/[/.]server\.[^/]+$/`) or a `server/` directory outside
  `src/routes`/`static` is rejected with a suggested replacement (`data.remote.ts`).
  `plan.md:L141-L149` (cites `src/exports/vite/utils.js:195-196`, `plugins/guard.js:113-122`)
- Ownership: every generated file has a header AND an inventory entry; generation never
  overwrites a target lacking both; path changes remove only the prior generator-owned file
  after the new one succeeds; app imports are ordinary TS the developer updates.
  `plan.md:L164-L169`, `worklog.md:L15`

### Generated TS shape

- Wrapper module: `import { command, query } from '$app/server'`; type-only import of the
  provider's `types.ts` (erased before Vite); `export type {...}` re-exports so consumers import
  types and functions from one module; `goRemoteOnly(name): never` throws; each export is
  `query('unchecked', (_input: In): Out => goRemoteOnly('name'))`,
  `command('unchecked', ...)`, `query.live('unchecked', async function* ...)`.
  `plan.md:L171-L201`
- Kit's client transform replaces the runtime exports with its own wrappers (including
  command `.updates(...)` and live proxies); the server bodies exist only as tripwires.
  `plan.md:L198-L201`
- Review confirmed the `'unchecked'` overload + callback signatures match the pinned Kit
  overloads and `RemoteLiveQueryUserFunctionReturnType`. `reviews/fable-plan-01.md:L43-L49`

### Stage A / Stage B

- **Stage A** (`skgo remotes generate [--check]`): scan + typecheck, resolve outputs, run the
  type provider per package (staged; see `typeglue.md`), write deterministic `.remote.ts` and
  `<app>/.skgo/remotes-source.json` (package import path, binding var, app-relative module,
  export, kind, types, refresh deps, every owned output). All committed; `--check` is CI.
  `plan.md:L205-L211`, `worklog.md:L19`
- **Stage B** (post-Kit join): a Vite plugin (`fixture/vite/skgo-capture.ts`) admits only
  inventoried modules; production loads each in the SSR environment and reads the file/hash from
  Kit's appended remote-init block (never computed); dev requests each module through Kit's
  client transform. Requires exactly one Kit address per discovered binding, **imported or
  not**. Writes `<go-root>/.skgo/remotes-kit.json` and
  `<go-root>/internal/skgoremotes/remotes_gen.go`, which imports the declaring packages,
  embeds the manifest, calls `Bind(module)` on every binding, exposes `NewHandler` and
  `RegisterRemotes`. `plan.md:L213-L237`, `worklog.md:L21`, `worklog.md:L25`
- Stage B artifacts are build-owned, gitignored, regenerated per mode (dev vs production
  transformed-code digests legitimately differ). `--check` is Stage A only. `plan.md:L239-L243`
- Capture authority stated once: production = Kit's SSR-observed module/hash; dev = Kit's actual
  client IDs; **kind always comes from Stage A**; the checker cross-checks
  `(module, export, kind, id)` tuples between modes. `plan.md:L225-L231`, `reviews/adjudication-02.md:L12-L15`
- Shipped capture mechanics (after Sol round 1): `buildStart` loader is a no-op so the first
  attempt captured nothing; the fix hooks `moduleParsed` and calls `this.load` on every
  inventoried module in both `ssr` and `client` environments — the ssr pass registers the
  remote with Kit and emits its server entry; the client pass emits the address. Dev:
  `transformRequest` per module with retry until Kit's lazy dev runtime can serve it.
  `junkyard/ephemeral/remote-codegen/implementation/generator-progress.md:L145-L158`
- Front doors: `skgo remotes generate --go-root ./backend --remote-root ./backend --app ./backend/app`
  and `skgo remotes build ... --main ./cmd/generated-candidate --out ./backend/bin/...`;
  `build` = Stage A -> `mise x -- vp build` with the capture plugin -> Stage B -> `go build`.
  `plan.md:L263-L275`, `junkyard/ephemeral/remote-codegen/validation/pr-body.md:L27-L32`
- Real ordering that works with no dummy registrar: Stage A generate -> Kit build (capture) ->
  Stage B -> `go build` of main. `go mod tidy` is unusable before Stage B (see traps).
  `implementation/generator-progress.md:L181-L207`

### Registrar shape

- Generated package `internal/skgoremotes`: imports declaring packages, embeds the manifest,
  `bindings := []remote.Binding{documents.GetDocument.Bind(module1), ...}`,
  `remote.NewHandler(manifest, bindings, version)`; `RegisterRemotes(mux)` mounts the whole
  Kit remote prefix (`base + "/" + appDir + "/remote/"`, recomputed locally because `Handler`
  exposes no `Prefix()`). `junkyard/ephemeral/remote-codegen/runtime-interface.md:L30-L60`,
  `implementation/generator-progress.md:L86-L90`
- Stage B validation errors: "no address from Kit for N declared remote(s)" and "N remote(s)
  reported by Kit with no Go declaration". `implementation/generator-progress.md:L221-L225`

### Topology used for proof (all sidecar/SSR-dependent)

- Dev: Vite/Kit serves the shell; production: adapter-node's `build/index.js` serves it; in
  both, the oracle "switchboard" forwards only the Kit remote prefix to the Go handler; Go also
  serves `GET /healthz`. `plan.md:L296-L301`
- Kit's Node server also serves `/_app/remote/*` for every remote in its server manifest, so a
  misrouted request reaches the Node tripwire; the gate asserts tripwire count zero.
  `reviews/fable-plan-02.md:L89-L94`

## Citations (bookmarks)

- Authored Go contract: `junkyard/ephemeral/remote-codegen/plan.md:L67-L122`
- Placement + Kit guard: `plan.md:L124-L169`
- Generated TS shape: `plan.md:L171-L201`
- Stage A / Stage B lifecycle: `plan.md:L203-L243`
- Kit-facts-that-constrain: `plan.md:L39-L65`
- Fixture tree + front-door commands: `plan.md:L245-L303`
- Implementation file map (`go/internal/remotegen/{scan,placement,registration,admit,provider,emit,generate,stageb}.go`):
  `implementation/generator-progress.md:L53-L67`
- Capture plugin behaviour as shipped: `implementation/generator-progress.md:L145-L158`
- Bootstrap ordering: `implementation/generator-progress.md:L181-L207`
- Handoff summary of what was final: `junkyard/ephemeral/remote-codegen/handoff.md:L1-L47`

## Reusable verdict against the new brief

New brief: marker functions (`var _ = skgo.QueryFunction(getUser)`) instead of wrapper
values; `*.remote.go` / `page.server.go` / `layout.server.go` colocated in the route tree with
symlinks making `[id]`/`(group)` dirs importable; stubs that throw; polytype; also emit
`+page.server.ts`/`+layout.server.ts`; Go serves the CSR bundle; no sidecar, no SSR.

| Element | Verdict | Notes |
|---|---|---|
| Kit is the sole identity authority; skgo never hashes | KEEP (principle) / KEEP WITH CHANGES (mechanism) | The old mechanism was a Vite plugin capturing from SSR + client transforms during a Kit **production build** and a running **dev server**. Neither exists as-is in the new design. Two viable replacements: (a) skgo's own SvelteKit adapter runs inside `vp build` and can read Kit's server manifest `remotes` map / postbuild metadata (`reviews/fable-plan-02.md:L45`, `:89-91` cite `index.js:1196`, `analyse.js:148-165`) and write a JSON the Go binary embeds; (b) reimplement Kit's `hash(root-relative-path)` (`src/utils/hash.js`) in Go and assert equality at build time. The old plan rejected (b) on principle (`plan.md:L51`), but reviewer noted the hash is the same function in both modes (`reviews/fable-plan-01.md:L106-L108`). Since the new brief has no dev Node server, option (a)+(b) as a cross-check is the pragmatic route. |
| Named remote exports only, type-only imports, `'unchecked'` overloads, throwing bodies | KEEP | Exactly "stubs that throw". Kit still needs the stub to exist as a real remote module so its client transform can assign the address and kind. Note dev-mode Kit *imports the server module* to learn kinds (`plan.md:L44-L45`), so the stub must be importable under Node without side effects — throw only inside the function body. |
| Kit server-only-name guard (`server.remote.ts` etc.) | KEEP | Real, runtime-proved trap with a misleading Kit error. `plan.md:L141-L149`, `reviews/adjudication-01.md:L6-L13` |
| `//skgo:remote output=` directive + `--remote-root` mapping | REDESIGN (likely drop) | With Go colocated in the route tree, the sibling rule (`document.remote.go` -> `document.remote.ts`) becomes the only rule. The directive and remote-root mapping were for non-colocated Go. |
| Sibling placement for in-app Go | KEEP | It was the user-directed default already (`reviews/adjudication-01.md:L21-L26`) and is now universal. Beware `remote_module_pattern` matching `.remote.go` (see traps). |
| Wrapper-value constructors `remote.Query("name", fn)` | REDESIGN | Replaced by marker functions. What transfers: resolve the marker by `go/types` package identity + name; read `In`/`Out` from the instantiated generic's type args (`types.Info.Instances`) or from the handler's signature; require named types; export name = Go func name (lowerCamel) instead of a string argument; reject duplicates per module. `plan.md:L98-L104`, `runtime-interface.md:L26-L28` |
| `RefreshRequested` static allow-list via `QueryRef` | KEEP WITH CHANGES | With marker functions there is no binding value to `.Ref()`; express refresh deps as a marker option (e.g. `skgo.CommandFunction(save, skgo.Refreshes(getDocument))`) and resolve the referenced func by `go/types` object identity. Runtime semantics (intersection with client-requested keys; unknown keys skipped) KEEP. |
| Stage A committed / Stage B ignored split | KEEP WITH CHANGES | The committed/ignored boundary was right. But generating Go **code** in Stage B caused the bootstrap trap (main imports a package that does not exist until after the Kit build). Prefer: registrar Go code is generated in Stage A (stable, mode-independent), and Kit addresses are **data** (embedded JSON written by the adapter at Kit build time), so the module always compiles. This is the fix fable-plan-01 finding 2 recommended (`reviews/fable-plan-01.md:L106-L109`). |
| Discovery scoped to remote packages + deps, never `./...` | KEEP | Directly enables first clean generation. `plan.md:L69-L76` |
| Ownership header + inventory + sha256 before deletion | KEEP | Hardened after a real path-traversal finding. `implementation/generator-progress.md:L167-L175` |
| Proof topology (oracle switchboard, adapter-node shell, dev Vite) | REDESIGN | Entirely sidecar/SSR-bound. New proof = Playwright against the Go server serving the CSR bundle. |
| `internal/skgoremotes` single registrar with `NewHandler`/`RegisterRemotes` | KEEP WITH CHANGES | The new brief wants per-function `http.Handler` wrappers mounted at Kit's URLs; a single aggregating registrar is still the natural mount point. |
| Loads (`page.server.go`) | NOT COVERED | Old design deferred loads entirely (`plan.md:L508-L510`). Kit's load-data endpoint (`__data.json`) and devalue-encoded load responses are new ground; nothing here to reuse beyond the codec. |
| `form` / `prerender` remotes | NOT COVERED | Deferred (`plan.md:L508`). Trap for the new brief: Kit executes `prerender` remotes at build time in Node — a throwing stub will break `vp build`; the generator needs a build-time strategy (see `lessons-and-traps.md`). |

### Where the old design depended on Node/SSR/production-build capture (must be replaced)

1. Stage B address capture = a Vite plugin inside Kit's **production build** (SSR env load) and
   a **dev server** transform request. `plan.md:L213-L231`, `implementation/generator-progress.md:L145-L158`
2. Dev proof = Vite dev server serving the shell + oracle switchboard routing the remote prefix
   to Go. `plan.md:L277-L285`, `plan.md:L296-L301`
3. Production proof = adapter-node `build/index.js` serving the shell. `plan.md:L297-L298`
4. The SSR case itself was explicitly out of scope (issue #9). `validation/closeout-issues.md:L16-L20`
5. Dev/production identity cross-check assumed both a dev capture and a production capture
   exist. `plan.md:L225-L231`

## Gotchas and traps (design-level; see lessons-and-traps.md for the full list)

- `server.remote.ts` is a Kit server-only module even with remote functions enabled; Kit's
  error text wrongly tells you to enable `experimental.remoteFunctions`. `reviews/adjudication-01.md:L6-L13`
- Kit's `remote_module_pattern` matches `document.remote.go`, so the dev watcher treats Go
  edits as remote-module changes. `reviews/fable-plan-01.md:L116-L119`, `reviews/adjudication-01.md:L24-L26`
- Dev Kit manifest is initialized lazily inside middleware; programmatic transform before the
  first HTTP request may run too early — warm up with one page request. `reviews/fable-plan-03.md:L96-L107`
- Vite `buildStart` loader is a no-op for capture; use `moduleParsed` + `this.load`. `implementation/generator-progress.md:L147-L150`
- Every plain-object argument arrives wrapped in Kit's `__skrao` devalue custom type. `implementation/generator-progress.md:L122-L131`

## Recipes

- To read the author-facing contract as a whole: start at `junkyard/ephemeral/remote-codegen/plan.md:L67`.
- To copy the `.remote.ts` stub template: `plan.md:L177-L196`.
- To list the exact Kit source lines that constrain remote identity: `plan.md:L39-L58` and `reviews/fable-plan-01.md:L27-L38`.
- To implement the Kit server-only-name guard: `plan.md:L141-L149`.
- To see how the Vite capture plugin was made complete (unimported modules): `implementation/generator-progress.md:L145-L158`.
- To see the working Stage A -> Kit build -> Stage B -> go build order and why tidy fails early: `implementation/generator-progress.md:L181-L207`.
- To see the CLI flag surface actually shipped: `validation/pr-body.md:L27-L32`, `implementation/generator-progress.md:L56`.
- To see the seven proof cases the gate ran (as a checklist for a Playwright rewrite): `plan.md:L439-L484`.
