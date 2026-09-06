# Junkyard remote-codegen: lessons, traps, and what reviewers got right

Purpose: concrete findings from the progress logs, worklog, reviews and closeout — what
actually broke while making a browser talk to Go through generated stubs, the Kit and Go facts
that were paid for, what remained open, and which review findings were real (vs. process
noise). Use this before designing the new generator; each item is a trap you would otherwise
rediscover.

## Kit facts learned (pin 3.0.0-next.25 / a593272)

- **Remote ID = `hash(root-relative posix module path) + '/' + export`**, same function in dev
  and production (`src/exports/vite/index.js:643-680`, `src/utils/hash.js`). Dev learns kinds by
  importing the server module; production reads analysed build metadata (`index.js:703-746`,
  `src/core/postbuild/analyse.js:148-165`); the server manifest has a `remotes` map (`index.js:1196`).
  `junkyard/ephemeral/remote-codegen/plan.md:L41-L47`, `junkyard/ephemeral/remote-codegen/reviews/fable-plan-01.md:L43-L45`,
  `reviews/fable-plan-02.md:L45`, `reviews/fable-plan-02.md:L89-L91`
- Kit rejects default exports and non-remote runtime exports from a remote module
  (`src/exports/internal/server/remote-functions.js:11-27`). `plan.md:L46-L50`
- **`server.remote.ts` is a server-only module** (`server_only_module_pattern = /[/.]server\.[^/]+$/`,
  `utils.js:195`; guard at `guard.js:113-122`), rejected even with remote functions enabled, and
  Kit's error wrongly says to enable `experimental.remoteFunctions`. Renaming to
  `data.remote.ts` fixed both dev and build. `reviews/fable-plan-01.md:L53-L81`,
  `reviews/adjudication-01.md:L6-L13`, `junkyard/ephemeral/remote-codegen/worklog.md:L17`
- `server_only_directory_pattern` (`utils.js:196`) excludes `src/routes` and `static`.
  `plan.md:L145-L149`
- **`remote_module_pattern` (`utils.js:144`) matches `document.remote.go`**, so Kit's dev
  watcher treats Go file edits as remote-module add/unlink events (`dev/index.js:414`) and
  Vite `/@fs` will serve the Go file. Accepted as harmless noise in a manual restart workflow.
  `reviews/fable-plan-01.md:L111-L122`, `reviews/adjudication-01.md:L21-L26`
- Kit's dev manifest initializes lazily inside middleware (`dev/index.js:526`,
  `init_manifest ??= update_manifest()`); programmatic transforms before the first HTTP request
  may run too early — issue one warm-up page request first. `reviews/fable-plan-03.md:L96-L107`
- Dev transform appends `import.meta.hot?.accept()` (`index.js:750-752`), so transformed-code
  digests differ between dev and production. `reviews/fable-plan-01.md:L90-L92`
- **Every plain-object argument is wrapped in Kit's `__skrao` devalue custom type**
  (`runtime/shared.js:85`, reducers 150-163, revivers 176-178); siblings `__skram`/`__skras`/`__skraf`
  exist for Map/Set/functions. `junkyard/ephemeral/remote-codegen/implementation/generator-progress.md:L122-L131`
- Kit's Node server serves `/_app/remote/*` for every remote in its server manifest, including
  ones force-loaded for capture. `reviews/fable-plan-02.md:L89-L94`
- The `'unchecked'` overloads for `query`/`command`/`query.live` with the plan's callback
  signatures are valid at this pin. `reviews/fable-plan-01.md:L43-L45`, `reviews/sol-typeglue-01.md:L15-L17`
- A refresh allow-list of client-requested keys executed after a successful command and
  returned in the same response mirrors Kit's `requested(...).refreshAll()`. `reviews/fable-plan-01.md:L46-L49`

## Vite/build facts learned

- A Vite plugin's `buildStart` loader is a no-op for capture: the first capture attempt
  recorded nothing. Hook `moduleParsed` and call `this.load` on every inventoried module in
  both the `ssr` and `client` environments; ssr registers the remote with Kit and emits its
  server entry, client emits the address. Emit nothing as an entry so no discovery chunk ships.
  `implementation/generator-progress.md:L145-L156`
- Dev: `transformRequest` on the client environment per not-yet-seen module, retrying until
  Kit's lazy dev runtime can serve it. `implementation/generator-progress.md:L152-L155`
- Overlaying only `src/` onto the app copy missed `vite.config.ts`; overlay the whole
  directory. `implementation/fixture-build-progress.md:L12-L16`
- An emitted browser entry importing every generated module (the plan's first idea) would ship
  dead client code and change Kit's output; capture from the SSR side instead.
  `reviews/fable-plan-01.md:L124-L141`

## Go facts learned

- **Registrar bootstrap trap**: `cmd/.../main.go` imports the generated `internal/skgoremotes`,
  which does not exist until after the Kit build. Before Stage B, `go build ./...` reports
  "updates to go.mod needed" and `go mod tidy` fails with a 404 finding the missing package.
  `go mod tidy -e` works as a workaround; the real fix used was pre-declaring the validator deps
  in `go.mod`/`go.sum` and never tidying before Stage B. `implementation/fixture-build-progress.md:L26-L54`,
  `implementation/generator-progress.md:L181-L207`
- First provider generation adds `santhosh-tekuri/jsonschema/v6` + `golang.org/x/text`; skgo
  refuses to edit `go.mod`. `implementation/generator-progress.md:L92-L96`
- Discovery must load only remote packages + deps, not `./...`, so a missing generated
  package cannot block its own creation. `plan.md:L69-L76`, `worklog.md:L13`
- go-gen-jsonschema's `-no-changes` rewrites `jsonschema_gen.go` on the no-drift path
  (`internal/builder/builder.go:96-124`); never trust it as read-only. `plan.md:L349-L354`
- A committed inventory JSON that is trusted blindly can delete files outside the roots
  (`../../outside.txt`). Fix: validate every inventory path as clean, relative, contained,
  non-duplicate; record sha256 per generated file; delete an obsolete output only when bytes
  match. `reviews/sol-generator-01.md:L57-L69`, `implementation/generator-progress.md:L167-L175`
- Obsolete-file ownership was checked after new files were written, so a refused delete leaves
  a half-applied rename; preflight all digests first (filed as issue #8).
  `reviews/sol-generator-02.md:L45-L54`, `junkyard/ephemeral/remote-codegen/validation/closeout-issues.md:L9-L14`
- `Handler.refresh` must skip unknown client keys when an allow-list is present (a Node-owned
  query in `.updates()` otherwise aborts a valid command). `reviews/sol-runtime-01.md:L29-L45`
- `httptest.ResponseRecorder` polled from another goroutine is a real data race; test SSE
  through `httptest.Server`. `reviews/sol-runtime-01.md:L47-L60`
- Marker calls must resolve by `go/types` package identity + name, never source text or import
  alias (copied from the provider's `ParseValueExprForMarkerFunctionCall`).
  `junkyard/ephemeral/remote-codegen/typeglue-source.md:L201-L204`
- `WithEnum` is for named strings; `WithStringerEnum` for iota ints with `String()`.
  `implementation/runtime-worklog.md:L163-L166`
- `go build -tags=jsonschema ./...` is the check that registration files compile.
  `implementation/sonnet-generator-task.md:L133-L137`
- Fixture layout that made every placement case reachable in one run: one backend Go module
  containing both `app/` and `remotes/`, scanned with `--go-root backend --remote-root backend
  --app backend/app`. `reviews/adjudication-02.md:L6-L10`, `plan.md:L245-L261`

## Process lessons that changed the design (real, not noise)

- Round 1 of plan review found the flagship example filename was a Kit server-only module and
  cases 1/4/5/6/7 could never go green; verified by a disposable Kit fixture.
  `reviews/fable-plan-01.md:L53-L81`
- Committed registrar cannot be drift-free because dev/prod manifests differ -> split Stage A
  (committed) from Stage B (ignored, per-mode). `reviews/fable-plan-01.md:L83-L109`,
  `reviews/adjudication-01.md:L15-L19`
- "Capture from the transformed client" and "capture from SSR" were both in the text with no
  authority order; fixed by stating production=SSR hash, dev=client IDs, kind=Stage A.
  `reviews/fable-plan-02.md:L71-L85`
- The 7/7 gate passed while never exercising an **unimported** remote module; the plugin only
  captured modules Vite happened to transform. Fixed by driving the inventory explicitly and
  adding `unused.remote.go`. `reviews/sol-generator-01.md:L29-L43`, `implementation/generator-progress.md:L145-L158`
- The gate recorded dev and production IDs but never compared them. Fixed by asserting equal
  sorted `module#exportName#kind#id` sets. `reviews/sol-generator-01.md:L45-L54`
- The SSR-ownership "failure" in the first live gate was a proof bug (exchange recorder
  finalized the live stream after inspection). `reviews/sol-generator-01.md:L21-L25`
- Sol's typeglue review correctly showed a bare `NewJSONSchemaMethod` marker cannot produce
  enums/unions; the resolution was to make registration developer-owned rather than narrow scope.
  `reviews/sol-typeglue-01.md:L23-L45`, `worklog.md:L44`, `worklog.md:L50`
- Sol's typeglue review also caught that `--check` did not cover the stub/schema/gen.go set and
  that root removal had no rule for them. `reviews/sol-typeglue-01.md:L48-L66`

## Open issues at closeout

- **#9 SSR Kit-to-Go bridge** — the slice was CSR-only; `+page.ts: export const ssr = false`.
  `validation/closeout-issues.md:L16-L20`, `implementation/generator-progress.md:L135-L136`
- **#8 preflight obsolete-file digests before writes.** `validation/closeout-issues.md:L9-L14`
- `remote.Handler` has no `Prefix()`; registrar duplicates the prefix computation.
  `implementation/generator-progress.md:L86-L90`, `:141`
- Forms, prerender, batch, loads, richer transport values, adapter/sidecar lifecycle: all
  unimplemented. `plan.md:L507-L510`
- Unresolved reviewer nits: filesystem-walk discovery descending into `node_modules`; placement
  undefined for in-app Go outside `src/lib`; `--main` path relative-to ambiguity.
  `reviews/fable-plan-03.md:L64-L94`

## Reusable verdict against the new brief

| Lesson | Verdict | Notes |
|---|---|---|
| Kit is the ID authority; capture, do not compute | KEEP WITH CHANGES | The capture ran inside a Kit production build + dev server. The new design has neither at runtime, but still runs `vp build`; skgo's adapter is the natural capture point (server manifest `remotes`, postbuild metadata). Consider computing `hash()` in Go as a cross-check only. |
| Never name a generated module `server.remote.ts` / under `server/` | KEEP | |
| `.remote.go` matches Kit's remote filename regex | KEEP (as a known trap) | Worse in the new brief because ALL Go lives under `src/routes`; also `page.server.go` matches `server_only_module_pattern` (harmless unless imported). Consider Vite `server.watch.ignored` for `*.go` in the adapter. |
| `__skrao` decode | KEEP | Mandatory. |
| Registrar bootstrap trap | KEEP (as the reason to change Stage B) | Emit compile-stable Go in Stage A; make Kit hashes data, not code. |
| Inventory containment + digests | KEEP | |
| Refresh intersection skipping unknown keys | KEEP | |
| Unimported module must still get an address | KEEP | In the new brief every `*.remote.go` handler should be mounted regardless of whether a page imports it, which means the address source must enumerate all stubs, not only the client graph. If capture reads Kit's server manifest, unimported stubs are still absent unless something imports them — the adapter may need to force-load every inventoried stub (as the old plugin did) or the hash is computed in Go. |
| Dev/prod identity equality assertion | KEEP WITH CHANGES | Only one mode exists at runtime now; keep the shape as "Go-computed vs Kit-observed" equality if the cross-check route is taken. |
| CSR-only page with all remote calls in `onMount` | KEEP | Exactly the new brief's runtime model. Top-level `await` in markup would run during SSR/prerender — still a trap if prerendering is ever on. |
| Prerender remote kind | NEW TRAP | Kit executes `prerender` remotes in Node at build time; a throwing stub breaks `vp build`. The old slice never faced this. Options: stub calls a running Go dev binary over HTTP at build time, or Go emits the prerendered JSON outputs itself. |
| Loads via `+page.server.ts` stubs | NEW GROUND | Kit's client fetches `__data.json` for server loads; with no SSR, Go must implement that endpoint and its devalue response format. Nothing in this segment covers it. |

## Gotchas and traps (condensed checklist)

1. `server.remote.ts` -> use `data.remote.ts`; guard basenames and `server/` dirs.
2. `__skrao` wrapper on every struct argument; reject `__skram`/`__skras`/`__skraf`.
3. `buildStart` cannot capture; use `moduleParsed` + `this.load` in ssr AND client envs.
4. Dev manifest is lazy; warm up with a page request before programmatic transforms.
5. Generated Go package imported by `main` before it exists breaks `tidy`/`build ./...`.
6. Provider `-no-changes` is not read-only.
7. Bare `*T` fields project non-null; reject them.
8. First provider run adds a Go dependency; plan for `go.mod`.
9. Trusting inventory paths = arbitrary file deletion; validate + digest.
10. Unknown client refresh keys must be skipped, not errored.
11. Do not poll `ResponseRecorder` across goroutines.
12. Overlay the whole app dir, not just `src/`.
13. A gate that records identities but never compares them is not a gate.

## Recipes

- To see every Kit source line the design leaned on, with reviewer verification: `reviews/fable-plan-01.md:L27-L49`, `reviews/fable-plan-03.md:L39-L47`.
- To reproduce the `server.remote.ts` rejection: `reviews/fable-plan-01.md:L64-L70`.
- To fix capture completeness for unimported modules: `implementation/generator-progress.md:L145-L158`.
- To get past the registrar bootstrap without a dummy package: `implementation/generator-progress.md:L199-L207`.
- To harden inventory cleanup: `implementation/generator-progress.md:L167-L175`, `reviews/sol-generator-01.md:L57-L69`.
- To see the full seven-case gate and per-case assertion counts (template for a Playwright suite): `plan.md:L439-L484`, `validation/final-01.md:L17-L44`.
- To see what "closeout" left open: `validation/closeout-issues.md:L1-L30`, `validation/pr-body.md:L49-L52`.
- Gate runner entry point (Node harness, do not copy the process): `junkyard/ephemeral/remote-codegen/check-slice.sh:L1-L18`.
