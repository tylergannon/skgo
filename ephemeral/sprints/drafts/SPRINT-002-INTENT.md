# Sprint 002 Intent: Remote functions in Go (`query`, `query.live`, `command`; hand-written stubs)

## Seed

plan sprint 002: remote functions in Go

Tyler's notes (2026-09-05, verbatim intent): implement `query`, `query.live`, and
`command`-style remote functions and actually see them working. Prove them via Playwright
BDD tests written in Gherkin against simple scenarios that exercise the required
functionality, against a **production build**, then merge. Do it in the best way you see
fit. Do not get caught up planning: find out how to build it, build it, prove it, fix it.
Do not gold plate or over-polish; get it to where it works (isn't broken) and is 90-95%
complete, then merge it.

(Tyler's compaction instruction. Brief context: the build plan's Phase A step (c)/(d) —
"mount a hand-written Go handler at one remote URL … called from a hand-written throwing
`.remote.ts` stub; one Playwright test proves Go answered" — was deliberately left out of
Sprint 001. This sprint is that slice, done properly: the remote-function protocol ported
to Go from pinned kit source, with real Go implementations behind an unmodified kit client.)

## Context

- **State of the repo after Sprint 001** (`ephemeral/sprints/SPRINT-001.md`, done, main
  `beb7819`): root package `skgo` has `NewDevProxy(target, logf)` and
  `NewStaticHandler(build fs.FS)`; `example/cmd/main.go` picks one by `--proxy`;
  `example/web` is a kit 3.0.0-next.25 app (`ssr = false`, routes `/`, `/about`,
  `/items/[id]`) built by the 20-line `skgo-adapter.js` into `build/` and embedded with
  `//go:embed all:build`; `example/e2e` is playwright-bdd with three scenarios run against
  `BASE_URL` in dev (overmind: project-local `vp dev` + Air) and prod (bare binary).
  No remote functions, no server loads, no codegen, no polytype yet.
- **Falsified early:** kit's Vite plugin emits client stubs for `.remote.ts` modules
  regardless of adapter (`__remote.query('worolc/getTodos')` appears in the client bundle
  through the skgo adapter). The server chunk `chunks/remote-<hash>.js` is never copied by
  the adapter and never runs. Therefore Go must answer `/_app/remote/<hash>/<name>` in both
  modes, and in dev it must answer them **before** proxying to `vp dev` (kit's dev server
  would otherwise execute the throwing stub body and 500).
- **Everything needed is source-derivable** (index route B): ids are a pure function of the
  file path; the wire format is devalue + kit's `__skra*` reducers + base64url; the response
  envelope, error/redirect shapes, headers and CSRF rule are all in pinned
  `runtime/server/remote-functions.js` and friends. The junkyard already got a real kit
  client talking to Go for query/command/live (`sources/junkyard/oracle/wire-protocol-facts.md`);
  its Go code is reference, not a base.
- **Codegen is NOT this sprint.** The brief fans out codegen and the project generator
  after the bare server; this sprint proves the protocol and the Go authoring surface with
  a hand-written stub, so Sprint 003 (`skgo gen-bindings`) has a working runtime to target.
  The stub must still be a real `.remote.ts` module that kit can execute in Node at
  transform/analyse time, with bodies that throw (brief: "we will ALWAYS know that we're
  getting a response from Go if we get a valid response").
- **Polytype** is at v1.0.0-rc.9 (`ephemeral/inspiration/reference/polytype/README-LOCAL.md`;
  module path `github.com/tylergannon/polytype`, `go tool polytype gen -target ./pkg
  --validate --typescript DIR`, fluent `polytype.Declare(T.Schema)`). Tracked docs still
  saying rc.5 (`sources/libs/polytype.md`, build plan §3) are stale on the version, not on
  the layering (devalue → JSON → `ValidateJSON` → `json.Unmarshal`).
- **Svelte MCP** (`list-sections`, `get-documentation`, `svelte-autofixer`) is registered for
  this project and must be used for every `.svelte` file (autofixer with `async: true`
  once `compilerOptions.experimental.async` is on). The `svelte-component-factoring` skill
  governs where remotes and state live in the example app: the leaf component that renders a
  list imports the query; the mutation's refresh sits next to the list it changes; route
  pages stay thin; async siblings under their own `<svelte:boundary>`.

## Pyramid Index

- L0: Port kit's remote-function protocol (`query` GET, `query.live` GET/SSE, `command` POST) to Go from pinned source with ported tests, expose a typed Go authoring API, mount it in front of both the dev proxy and the prod static handler, and prove it with a hand-written throwing `.remote.ts` stub plus Playwright scenarios where every valid response can only have come from Go.
- L1:
  - **`query.live`:** SSE response per `create_live_query_response` (frames `data: {"type":"result","result":<devalue>}\n\n`, dedupe on serialized string, 30 s `: keep-alive` comment, `text/event-stream`, teardown on client disconnect via `context.Context`); Go authoring shape is a generator-like `func(ctx, In, yield func(Out) error) error` or a channel; port the two upstream spec tests (`sources/kit/portable-tests/remote-functions-spec.md`) through a real `httptest.Server`, never `ResponseRecorder`.
  - **Library ports (Go, test-first):** kit `hash` (golden `src/lib/todos.remote.ts → worolc`); devalue `stringify`/`parse` with the upstream fixture table minus JS-only rows; remote-arg codec (`__skrao`/`__skram`/`__skras` revivers, base64url, sorted-key canonical encoder); the `/_app/remote/<hash>/<name>` dispatcher: envelope `{type:'result',data:<devalue>}`, `q` node on every query response, `refreshes` allow-list + `q`/`r` on commands, error envelope `{status,message}` with HTTP 200, redirect-as-result, 404 inside the envelope, `cache-control: private, no-store`, Origin check on non-GET (`403 {"message":"Cross-site remote requests are forbidden"}`).
  - **Authoring surface:** `skgo.Query[In,Out]` / `skgo.Command[In,Out]` (or equivalent) taking `func(context.Context, In) (Out, error)`, a registry that mounts by `(hash, name)`, Go error → kit `error(status, message)` and `redirect` mapping, single-flight refresh from inside a command (the Go equivalent of `requested(...)`/`.refresh()`/`.set()`).
  - **Composition:** one handler ordering used by `example/cmd`: remote prefix → Go; everything else → proxy (dev) or static (prod). `base`/`appDir`/`origin`/`version.name` come from the adapter manifest so Go and the client bundle agree.
  - **Example app + proof:** `experimental.remoteFunctions` + `compilerOptions.experimental.async`; a hand-written `*.remote.ts` stub with `query('unchecked', …)`/`command('unchecked', …)` bodies that throw; Go implementations of a small todo/guestbook surface (list, get-by-arg, add with `.updates()` single-flight, one error path); Svelte pages using `await getTodos()` inside `<svelte:boundary>`; playwright-bdd scenarios (list from Go, single-flight command = exactly one programmatic request, arg query via deep link, error surfaces) green in dev and prod; a build-time tripwire that the id Go mounts appears in the built client bundle.
  - **Risks:** byte-fidelity of devalue (dedupe, number formatting, `<` escaping, key order); dev interception ordering vs Vite `/@fs` module fetches of the stub source; the stub executing in Node at transform/analyse (top level must not throw, no default export); CSRF origin under dev vs prod; scope creep into live/batch/form/prerender/loads.
- L2:
  - Ids/hash → `sources/kit/remote-server/ids-and-build.md`, `sources/kit/remote-client/client-transform.md`, `themes.md` T1.
  - Wire format → `sources/kit/remote-server/serialization.md`, `sources/kit/remote-client/client-requests.md` (worked payload examples), `sources/libs/devalue.md`, `sources/kit/portable-tests/devalue-port.md`.
  - Dispatcher/envelopes → `sources/kit/remote-server/request-handling.md`, `sources/kit/remote-client/client-expectations.md`, `sources/kit/portable-tests/remote-functions-spec.md`.
  - Stub shape → `sources/kit/remote-client/stub-shape.md`; dev interception → `sources/kit/build-adapt/dev-server.md`, `themes.md` T4.
  - Junkyard verdicts → `sources/junkyard/remote-codegen/runtime-interface.md`, `lessons-and-traps.md`, `sources/junkyard/oracle/wire-protocol-facts.md`.
  - Scenarios to borrow → `sources/kit/test-apps/remote-scenarios.md` §1, `sources/junkyard/app/guestbook-app.md`, `e2e-suite.md`.

## Semantic Index

- **Available** — Token cache: `/Users/tyler/src/skgo/ephemeral/inspiration/` (in this
  worktree also reachable as `ephemeral/inspiration/`, a symlink). Index entrypoint:
  `ephemeral/semantic-index/README.md` (config pointer: `ephemeral/SEMANTIC-INDEX.md`; this
  repo has no `docs/`). Access: read the entrypoint, then `TAXONOMY.md` route
  **B. Implement the remote-function protocol in Go** in the order listed, then `themes.md`
  T1–T4, T7, T8; open the cited leaves; `rg` the token cache only where a leaf points.
  Citations are relative to the token cache root (e.g.
  `reference/kit/packages/kit/src/runtime/shared.js:L84-L173`).
  Prior art retrieved (summarised in Context and the Pyramid Index): hash golden values
  (`worolc`, `mxe8u8`, `txkpmq`, `2rsbgs`, `16ghqs9`, `1oadct1`); worked payload examples
  (`1 → WzFd`, `"hi" → WyJoaSJd`, `{filter:'author:santa'} →
  W1siX19za3JhbyIsMV0seyJmaWx0ZXIiOjJ9LCJhdXRob3I6c2FudGEiXQ`); the query-response rule that
  the client reads `q[<hash>/<name>/<rawPayload>].v`, not `_`; command single-flight via
  `refreshes` + `q` + `r`; error envelopes at HTTP 200; junkyard traps (`__skrao` on every
  object argument, `server.remote.ts` is server-only, `*.remote.go` matches kit's remote
  regex, unknown refresh keys must be skipped, `ResponseRecorder` races in streaming tests).

Planning agents drafting this sprint must read the index entrypoint, follow route B, and
open the cited leaves before proposing implementation approaches. Do not scan the token
cache; sniff via the index. Do not read `ephemeral/inspiration/junkyard/ephemeral/**`
directly — its process artifacts are indexed only for traps. Read
`ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md` before writing
any SvelteKit code, and check pinned kit source over any documentation site.

## Chapter Context

No chapter link selected (this repo does not use chapters).

## Recent Sprint Context

- **Sprint 001 (done):** bare Go server fronting the kit app in dev and prod. Deviations
  from its plan are recorded in `ephemeral/worklog/202609052129-sprint-001-execute.md` and
  matter here: `builder.writeJson` does not exist (adapter uses `node:fs`); `NewDevProxy`
  takes a logger; `go test ./...` does not cross into `example/` (gate is
  `go test ./... ./example/...`, same for vet); `vp dev` must be the project-local binary
  with `vite` aliased to `@voidzero-dev/vite-plus-core`; Air's `root` only scopes the
  watcher; Air ignores `touch`; playwright-bdd fixtures extend `test` from `playwright-bdd`;
  reviewers reading only `runtime/props.svelte.js` wrongly conclude pages get no `params`
  prop (they do, via `PageProps`).

## Relevant Codebase Areas

```
go.mod / go.work                 module github.com/tylergannon/skgo; use . ./example
proxy.go, static.go (+tests)     Sprint 001 handlers (package skgo)
example/cmd/main.go              --listen, --proxy; X-Skgo-Mode header
example/web/skgo-adapter.js      writes build/client, build/index.html, build/skgo.manifest.json
                                 {appDir, base, version, routes[{id,pattern}]}
example/web/vite.config.ts       sveltekit({ adapter: skgo(), paths: { origin } }) — needs
                                 experimental: { remoteFunctions: true } and
                                 compilerOptions: { experimental: { async: true } }
example/web/src/lib/, src/routes/ the app; Greeting.svelte; +error.svelte uses $app/state
example/web/dist.go              //go:embed all:build (natural home for the id tripwire test)
example/e2e/                     playwright-bdd; steps/fixtures.ts counts document requests
ephemeral/inspiration/reference/kit/packages/kit/src/
  utils/hash.js                          djb2 backwards, base36
  runtime/shared.js (+ shared.spec.js)   __skra* reducers/revivers, base64url, remote keys
  runtime/server/remote-functions.js     dispatcher, collect_remote_data (+ spec: live only)
  runtime/server/errors.js, csrf.js      App.Error shaping, is_remote_forbidden
  runtime/client/remote-functions/       what the browser sends and expects
  runtime/app/server/remote/{query,command,shared}.js  factories, 'unchecked' validator
ephemeral/inspiration/reference/devalue/src/{stringify,parse,utils,constants}.js + test/index.test.js
```
Where new Go lives is a design point for the drafts (e.g. `internal/remote`,
public API in package `skgo`; the devalue codec itself is now
`github.com/tylergannon/polytype/devalue`, outside this repo), as is whether the typed layer goes
devalue → JSON → `json.Unmarshal` (junkyard, keeps polytype codecs and `ValidateJSON`
working) or devalue → Go reflection directly.

## Constraints

- AGENTS.md/CLAUDE.md rules: Go library, `go test`, no JavaScript except where kit requires
  it (the stub `.remote.ts`, `vite.config.ts`, app code, the Playwright suite); build the
  software, not proof machinery; worklog for actionable intelligence only.
- No SSR, no Node in production, Go never spawns Node. Stubs throw; Go answers.
- Kit 3 facts: no `svelte.config.js`; flat `sveltekit({...})`; `#lib`; `$app/server`
  import path for `query`/`command`; origin fixed at build (`paths.origin` =
  `http://127.0.0.1:8080`) so Go's Origin check must use that value; TypeScript 6.x.
- Port by direct observation of pinned source and port its unit tests; run a Node app
  only where reading is not enough (Tyler's addendum).
- Proof = playwright-bdd against `BASE_URL`, protocol-agnostic (DOM, counts of
  programmatic requests, document loads), never asserting on envelope bytes (`themes.md` T8);
  plus Go unit tests with golden vectors for the codec and envelopes.
- Every `.svelte` file passes `svelte-autofixer`; component ownership per
  `svelte-component-factoring`.
- Sprint docs in `ephemeral/sprints/`; ledger `ephemeral/sprints/ledger.yaml`
  (id `SPRINT-0002`, file `SPRINT-002.md`). Sprint size: one focused agent session.

## Success Criteria

1. `go test ./... ./example/...` and `go vet` pass; the devalue fixture port, hash goldens,
   payload goldens from `client-requests.md`, and envelope table tests are green.
2. In prod (bare binary from `vp build` + `go build`, Vite stopped) — dev is a bonus, not the gate — a browser at the Go
   origin renders a list fetched through `await getTodos()`, adds an item through a
   `command` with `.updates(getTodos())` making exactly one programmatic request, opens a
   deep link that calls a query with an argument, and shows an error state when a query
   throws, and a `query.live` subscription that updates the page as Go pushes new values — with the `.remote.ts` bodies throwing, so every rendered value came from Go.
3. Playwright BDD (Gherkin) scenarios for those flows pass against the production build via `BASE_URL`.
4. A test proves the `(hash, name)` ids Go mounts are present in the built client bundle.
5. No `query.batch`, `form`, `prerender`, `__data.json`, or generated code in this sprint;
   their absence is stated, not hand-waved. Bar: works, not broken, 90-95% complete; no gold plating.

## Open Questions

1. **Types for the hand-written stub:** hand-write `types.ts` this sprint, or introduce
   polytype rc.9 now (`//go:build jsonschema` file, `go tool polytype gen --validate
   --typescript`) so the stub imports generated declarations and Go validates with
   `ValidateJSON`? Recommendation to argue for or against: include polytype, because
   Sprint 003 codegen needs exactly this pipeline and the JSON bridge is cheap to prove now.
2. **Typed bridge:** devalue → JSON bytes → `json.Unmarshal` (junkyard-validated; works with
   generated codecs) vs a direct devalue↔Go reflection codec. What does `time.Time` become
   (`["Date", iso]` vs RFC3339 string)?
3. **Authoring API shape:** generic `skgo.Query[In,Out](name, fn)` returning a binding vs the
   brief's marker-function style `var _ = skgo.Query(getTodos)` (which only pays off with
   codegen). How does a Go handler express `error(404, "…")`, `redirect(303, "/x")`, and a
   server-driven refresh/set of another query inside a command?
4. **Where the stub lives:** `src/lib/todos.remote.ts` (hash `worolc`, golden already
   known) vs a route-colocated `src/routes/todos/data.remote.ts`. Never `server.remote.ts`.
5. **Dev CSRF:** kit skips the Origin check in dev; Go answers in both modes. Enforce in
   both (origin is fixed at build) or mirror kit (dev-off)?
6. **Version header:** emit `x-sveltekit-version` from `skgo.manifest.json`'s `version`
   (set `version.name` explicitly in `vite.config.ts` so dev and prod agree) or omit it?
7. **Sprint 003 seam:** which parts of this sprint's Go API must stay stable so
   `gen-bindings` only emits calls into it (registry, bindings, hash), and which are
   example-only?
