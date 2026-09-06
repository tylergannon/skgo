# Junkyard plan facts: what the earlier attempt decided, learned, and asked

## Purpose

`junkyard/` (`sveltekit-adapter-go`, restarted 2026-09-01, plan written 2026-09-02) never
produced a working server, but it paid for several facts and a proof method. This file keeps
the decisions and findings that still apply to skgo *as now framed* (CSR-only, Go serves the
bundle, no JS runtime in production, `vp dev` proxied in dev), lists the open questions it
put to Tyler (none were answered in the junkyard: no `NNNN.answer.md` files exist under
`junkyard/ephemeral/projects/skgo/interview/`), and names what should consciously **not** be
resurrected. It skips the process machinery (ledgers, sprint YAML, check scripts, proof
directories) except where a concrete finding hides inside.

Citation paths are relative to `/Users/tyler/src/skgo/ephemeral/inspiration/`.

## Key facts

### The product framing that survives

- End-state bullets: one binary + one build gesture (`skgo build` = `vp build` → embed →
  `go build`; stale frontend fails loudly; dev proxies Vite with HMR while Air rebuilds Go);
  TS surface indistinguishable from hand-written kit; Go surface is `http.Handler` +
  `context` + options structs; loads and remote functions in Go with end-to-end types, all
  else plain kit or plain Go; never reimplement kit, build a JS runtime, or claim what wasn't
  demonstrated. `junkyard/README.md:L9-L29`.
- Design that has **changed**: the junkyard supervised a Node sidecar running kit's server
  for SSR and proxied dynamic requests to it (`junkyard/README.md:L31-L39`;
  `chapters/03-proxy-host`, `05-supervisor`). skgo now is CSR-only with no JS runtime in
  production, so the sidecar, proxy, supervisor, and "Go speaks to kit's server in the
  sidecar" bridge (`chapters/09-go-remote/CHAPTER.md:L26-L31`) are gone. What the junkyard
  called the "native path" (`chapters/10-native/CHAPTER.md`: Go answers browser calls to
  Go-authored remote functions and serves `__data.json`) is now the **only** path.
- The dev proxy survives: `skgo dev` runs the Go host in front of Vite's dev server with
  WebSocket upgrade for HMR (`chapters/07-cli/CHAPTER.md:L5-L22`).

### Proof method (the strongest thing in the junkyard)

- **One real app, one Playwright task suite, pointed at whichever server `BASE_URL` names.**
  Level 0 = adapter-node (proves the tests), later levels swap a layer under the unchanged
  suite; a failure names the layer by construction. `junkyard/README.md:L41-L59`;
  `brief.md:L8-L23`. Level 0 was green: 23 tasks, locally and in hosted CI (`brief.md:L23`).
- **Protocol-agnostic assertions only**: DOM, navigation, document reloads, count of
  programmatic requests, public asset paths, response headers — **never** `__data.json`
  shapes, remote envelopes, devalue encodings, or `/_app/remote/*` URLs, which are
  unpublished and change without notice. This rule caught two of the author's own wrong
  assertions on the reference run. `junkyard/AGENTS.md:L33-L36`; `brief.md:L25-L39`.
- Proof = the suite green against an identified target with exact counts (`expected` equals
  the `--list` count, zero `unexpected`), plus identity (HEAD, binary SHA-256, versions, the
  pid that owns the port), nonces the target can't know in advance, and a written scope
  check. Unit tests/vet/lint are required checks, not proof. `brief.md:L41-L70`;
  `proof/FORMAT.md:L1-L20`.
- **The suite must bite**: negative controls (an always-200 stub, a naive static file server
  on the `@static` subset, a target that never listens) must make the check go red before a
  green is believed. `brief.md:L63-L65`; `chapters/01-referee/sprints.md:L8-L11`.
- Tags select tasks per target: untagged = every level; `@static` = servable with no Node
  (assets, immutable caching, etag, precompression, prerendered page + 308); `@host`,
  `@native` were level-specific. `brief.md:L72-L86`.
- **Running the suite without npx**: `mise x -- node node_modules/@playwright/test/cli.js
  test`; without `BASE_URL` Playwright builds and starts adapter-node on
  `http://127.0.0.1:4173`. `junkyard/README.md:L93-L107`.
- The suite ran one Playwright worker because the guestbook store was process-global
  (`chapters/01-referee/CHAPTER.md:L47-L49`).

### The task ledger (a feature checklist for skgo's example app)

Green at level 0 — 23 tasks (`junkyard/README.md:L61-L91`):

- Remote functions: `query` renders server data; `command` updates the query in a single
  flight (one programmatic request, no reload); `query.live` pushes an update made from
  another tab; `form` uploads a file without a reload, and again with JS disabled via the
  page-reload fallback; a `prerender` function renders its banner.
- Loads: nested layout + page server loads compose and persist a cookie; a streamed load
  shows the fast part before the slow part; a custom `transport` codec round-trips a class
  through a load and a query.
- Routing: SSR entry; hydration makes no programmatic request; client navigation never
  reloads; deep link and client nav render the same dynamic page; 404 for unknown route and
  unknown id; prerendered page served and its trailing slash redirects; a redirect thrown in
  a load lands on the destination.
- Forms (classic actions): validation failure with input preserved, login redirect, logout —
  with and without JS.
- Errors: expected error renders status + message; unexpected error renders 500 and leaks
  nothing.
- Platform: app sees its own URL, client address, hook-set locals, and the hook's response
  header; static assets have correct content type, immutable caching, etag revalidation,
  precompressed negotiation; oversized bodies → 413.

Under CSR-only, the SSR-entry, hydration, no-JS `form` fallback, and classic-action tasks
change meaning or drop; the rest are directly reusable claims.

### Kit-3 traps the junkyard hit (cross-ref `kit3-facts.md`)

- **Origin/CSRF trap**: kit 3 fixes the origin at build (`paths.origin`, set from an `ORIGIN`
  env var in `vite.config.ts`); every target must serve at that origin or browser POSTs get
  403. Interview 0001 recommends one fixed origin (`http://127.0.0.1:4173`), targets run one
  at a time, check script refuses an occupied port. `junkyard/README.md:L103-L107`;
  `interview/0001.md:L1-L19`; `SKILL.md:L57-L63`.
- Prerendered remote results are fetched over HTTP from the app's own origin during SSR, so
  "the host must answer `/_app/remote/*` for those files". `chapters/03-proxy-host/
  CHAPTER.md:L36-L39`. (Under CSR-only there's no SSR fetch, but the client still fetches
  them; Go serves them as static.)
- `transport` misplaced in `hooks.server.ts` stalled a work item. `SKILL.md:L34-L39`.
- Adapter API reads config from `config` directly, not `config.kit`. `chapters/02-adapter/
  CHAPTER.md:L30-L33`.
- `npm`/`npx` refuse to run once `vp install` pins pnpm via `devEngines`. `junkyard/AGENTS.md:
  L38-L41`.
- TS 7 breaks kit sync; root TS 6.0.3 is the fix. `SKILL.md:L103-L108`.

### Adapter / build-output design that still applies

- The adapter is an ordinary kit adapter; output goes to a directory a Go package embeds
  (Go `embed` can't reach above the package dir, so the guestbook set
  `out: '../cmd/guestbook/build'`). `chapters/02-adapter/SPRINT-01.md:L5-L14`, `L37-L42`.
- Proposed output tree: `client/` (`builder.writeClient`), `prerendered/`
  (`builder.writePrerendered`, includes prerendered remote results), precompressed `.br`/`.gz`
  beside every compressible file (`builder.compress`), plus a manifest. Under CSR-only the
  `server/` bundle is not needed at runtime. `SPRINT-01.md:L16-L21`.
- Proposed `skgo-manifest.json` (keys nothing else read were forbidden):
  `schemaVersion`, `kit` (exact installed version), `devalue`, `adapter`, `origin`
  (`config.paths.origin` or null), `appDir` (`_app`), `assets` (every client file, relative),
  `prerendered` (paths as served, incl. `/_app/remote/…/getBanner`), `entry`.
  `SPRINT-01.md:L23-L35`; Go `Manifest` struct + `Load(fs.FS)` with instruction-style errors
  `SPRINT-02.md:L6-L24`.
- **Validated kit pair gate**: the adapter carries a data list of `{ kit, devalue }` pairs it
  was proven against and fails the build for any other pair, with `SKGO_UNVALIDATED_KIT=1`
  as a loud escape hatch; read devalue's version from kit's resolved dependency.
  `SPRINT-03.md:L1-L35`. This is how "kit releases are absorbed by a pipeline, never a
  migration" was meant to be enforced (`chapters/11-release/CHAPTER.md:L5-L21`: feed-triggered
  bump → level 0 → later levels → PR with proof and a corrected kit-facts skill).
- `ADAPTER=node|skgo` env switch in `vite.config.ts` kept level 0 on adapter-node while the
  skgo adapter was built. `chapters/02-adapter/CHAPTER.md:L52-L54`.
- Static serving reference: adapter-node's `sirv` configuration (in `reference/`) for
  content types, immutable caching on `_app/immutable`, etag/304, precompressed negotiation
  with `Vary`, trailing-slash 308; dispatch order static → prerendered → dynamic; body limit
  → 413. `chapters/04-static-native/CHAPTER.md:L5-L24`, `L40-L46`; `chapters/03-proxy-host/
  CHAPTER.md:L22-L25`.
- Host API shape that was planned: `New(fs, opts...)`, `Handler()`, `HandleExtra(pattern, h)`
  failing start on a shadowed app route, `Project(func(*http.Request) Locals)`,
  `Shutdown(ctx)`, `ListenAndServe` honouring `PORT`/`HOST`/`SOCKET_PATH` (adapter-node
  parity). `chapters/06-host-api/CHAPTER.md:L5-L22`.

### Go remote-function codegen facts (the junkyard got this far)

- `skgo` scanned `*.remote.go` files for functions wrapped in `remote.Query`/`remote.Command`/
  `remote.Live` with named input/result types and generated the matching `.remote.ts`
  modules, TS declarations, and a single Go registrar, using the developer's build-tagged
  `go-gen-jsonschema` registrations (`WithEnum`, `WithStringerEnum`, `WithInterface`) for
  enum/union codecs. `junkyard/README.md:L114-L121`.
- `skgo remotes build` ran the app's **real kit production build to capture kit's real
  remote addresses** before generating the Go registrar. `junkyard/README.md:L121-L125`.
- The slice was explicitly **client-only (CSR)**: a page importing a generated remote during
  SSR tripped a clear generated error. `junkyard/README.md:L125-L128`. This is exactly skgo's
  current model.
- **Route-colocated Go**: `*.remote.go` may live under `src/routes` (inside `(group)` and
  `[id]` dirs). Because those names aren't valid Go path elements, `skgo` made each colocated
  package importable via a symlink at `<GoRoot>/.skgo/links/<base32-of-relative-path>` and
  created `<AppDir>/src/routes/go.mod` as a module boundary so bracketed/parenthesised dirs
  never fall inside the enclosing module's `./...`; both recorded in `.skgo/links.json`.
  `skgo test` wrapped `go test`, appending `./<link>` for each entry. Validated with
  `gopls check` on macOS arm64, Go 1.27.1, no `go.work` needed. `junkyard/README.md:L133-L172`.
- Generated type surface seen working: `remote.QueryBinding[GetMessageInput, Message]`.
  `junkyard/README.md:L166-L168`.
- Planned but unbuilt: `query.batch`, `form` with file upload, `prerender` in Go; `query.live`
  as a Go channel/iterator with cancellation; validation from Go types (struct tags vs schema
  library vs generated Standard Schema). `chapters/09-go-remote/CHAPTER.md:L5-L45`.
- Planned Go devalue port with devalue's own test suite ported as a required check (devalue
  is a published library with a public contract — the one allowed "ported suite").
  `chapters/10-native/CHAPTER.md:L22-L25`, `L43-L46`.
- Chapter 8's bridge plan for loads: a Go `event` type (params, url, locals projection,
  cookies read), generated TS declarations per Go unit so `data` is typed, errors/redirects
  from Go behaving as `error()`/`redirect()`; universal loads out by definition.
  `chapters/08-go-loads/CHAPTER.md:L13-L23`, `L46-L49`.
- The `oracle/` laboratory held source-derived protocol fixtures for query/live/command; the
  rule was "native kit executions at the pin remain the reference; stored observations are
  evidence, never the sole oracle". `junkyard/README.md:L174-L179`.

### Interviews (all unanswered; recommendations recorded)

| # | Question | Recommendation |
|---|---|---|
| 0001 | How to serve every target at the one build-time origin | One fixed origin `http://127.0.0.1:4173`, targets run one at a time, port guard. `interview/0001.md` |
| 0002 | Where host-only claims (sidecar kill, drain) are proven | Go integration tests against the real binary + Playwright `@host` only for browser-visible effects. `interview/0002.md` (sidecar no longer exists; the split "Go tests for process behaviour, Playwright for what the app sees" still holds) |
| 0003 | Repo shape | One repo, `adapter/` (npm) + `go/` (module, `cmd/skgo`), `app/` as the proving app; module path may move to root at ship. `interview/0003.md` |
| 0004 | What "typed end-to-end" means | **Go is the source; TS declarations and `.remote.ts` shims are generated; validation derives from Go types.** `interview/0004.md` |
| 0005 | Are classic form actions written in Go | No — actions stay TS; remote `form` is kit's direction. `interview/0005.md` (under CSR-only with no Node, actions can't exist at all) |
| 0006 | CLI before or after Go backend | Finish the CLI first so level-2 proofs ride the shipped build gesture; option 3 (minimal `skgo build` early) is the compromise. `interview/0006.md` |
| 0007 | The idiomatic-Go gate | golangci-lint + modernize as command, a rubric-driven model judge that didn't write the code, plus Tyler's read of the exported surface. `interview/0007.md` |
| 0008 | A live wire tap for attribution | Not now; Playwright traces on failure suffice and a tap tempts protocol assertions back in. `interview/0008.md` |

Other open decisions named in chapter docs and never resolved: sidecar entry bundled vs
shipped (`02-adapter:L45-L48`, moot); manifest asset list with hashes now or later
(`SPRINT-01.md:L61-L63`, recommended later for the staleness check); `HEAD`/`Range`/
`If-Modified-Since` parity with sirv vs Go `http.ServeContent` (`04-static-native:L42-L46`);
the 502 contract (moot); `Locals` projection type — `map[string]any` vs struct vs generics
(`06-host-api:L40-L43`); `skgo build` embedding via generated package vs `go:embed` of the
adapter output dir (`07-cli:L44-L45`); which loads move to Go first and whether the layout
load moves (`08-go-loads:L40-L44`); `query.live` mapping and cancellation
(`09-go-remote:L43-L45`); the conservative rule for native `__data.json` — all-Go routes
only vs mixed with a TS layout (`10-native:L41-L44`; now every route is "all-Go" or plain
client); whether the devalue port is a separate module (`10-native:L45-L46`).

## Citations

- `junkyard/README.md:L1-L179`
- `junkyard/AGENTS.md:L1-L46`
- `junkyard/ephemeral/projects/skgo/brief.md:L1-L140`
- `junkyard/ephemeral/projects/skgo/BUILD.md:L1-L73` (process; toolchain lines L39-L46)
- `junkyard/ephemeral/projects/skgo/chapters.md:L1-L100`
- `junkyard/ephemeral/projects/skgo/chapters/{01..12}-*/CHAPTER.md`
- `junkyard/ephemeral/projects/skgo/chapters/01-referee/SPRINT-01.md:L1-L45`
- `junkyard/ephemeral/projects/skgo/chapters/02-adapter/SPRINT-0{1,2,3}.md`
- `junkyard/ephemeral/projects/skgo/interview/000{1..8}.md`
- `junkyard/ephemeral/projects/skgo/proof/FORMAT.md:L1-L20`
- `junkyard/.agents/skills/sveltekit-current/SKILL.md:L1-L123`

## skgo implications

- **Keep the proof shape, drop the ladder.** One example app, one Playwright suite driven
  by `BASE_URL`, protocol-agnostic assertions, exact counts, a negative control. With no
  sidecar there are two targets that matter: kit's own `vp preview`/adapter-node build of
  the same app with TS backends (proves the tasks) and the Go binary (proves skgo). Run
  them at the same fixed origin, sequentially.
- **Origin is a first-class config of `skgo build` and `skgo dev`**: the Go listener, the
  `paths.origin` passed to kit, and the CSRF allowlist must agree; surface a single setting.
- **Adopt the codegen direction**: Go source of truth; `*.remote.go` colocated under
  `src/routes` with the symlink + `src/routes/go.mod` boundary trick; `.remote.ts` shims
  emitted next to them; the TS stub throws; kit's build output is read to learn real remote
  addresses; `go-gen-jsonschema` for enum/union codecs.
- **Reuse the ledger as the example-app feature list**, re-scoped for CSR: query, batch,
  live (cross-tab push), command single-flight (exactly one programmatic request), enhanced
  form with file upload, prerender banner, nested layout+page Go loads with a cookie, a
  streamed load (if supported), a `transport` codec round-trip, deep-link vs client-nav
  parity, 404s, prerendered page + 308, redirect from a load, expected vs unexpected error
  rendering, own-URL/client-address/locals visibility, static asset headers, 413.
- **Static serving contract** comes from adapter-node's sirv config in `reference/`, not from
  guesses; dispatch static → prerendered → app shell / data / remote.
- **Kit-bump policy**: an adapter-side validated-pair check with an explicit override, and
  the kit-facts file updated on every bump.
- **Idiomatic-Go gate**: lint + modernize + an independent reviewer with a written rubric
  before the public API is called done.

## Gotchas

- Everything in the junkyard that mentions sidecar, proxy, supervisor, `x-skgo-*` headers,
  502-with-request-id, Node discovery, SSR entry, hydration, or "Go speaks to kit's server"
  is **obsolete** under the CSR-only framing. Don't port it.
- The junkyard's guestbook used classic `+page.server.ts` actions for login (`interview/
  0005.md:L3-L4`); under CSR-only with no Node there is no runtime for them. The example app
  must reimplement login as a remote `form` (Go) or the task drops.
- "Level 0 green" was against **SSR** adapter-node; a CSR-only reference build changes what
  some tasks observe (no SSR entry, hydration request counts differ). Re-derive the task
  list; don't copy assertions.
- The `oracle/` fixtures were explicitly not an oracle; kit at the pin is. Don't treat any
  recorded wire bytes as truth.
- History before 2026-09-01 (a conformance harness and its spec pile) is under tag
  `snapshot-2026-09-01-pre-purge` — the junkyard itself said not to resurrect it
  (`junkyard/AGENTS.md:L44-L46`).

## What NOT to resurrect

- Chapter ledgers with YAML frontmatter, `done: true` counters, `check-sprint-*.sh`,
  `check-level.sh`'s target-hook framework, proof directories/READMEs, `infer` judges in
  ledgers, the twelve-chapter vector (`chapters.md`, `BUILD.md`, `proof/FORMAT.md`). They
  were built *instead of* the product (this repo's `CLAUDE.md` says the same).
- The sidecar/proxy/supervisor chapters (03, 05) and the level-1 host API's sidecar-shaped
  parts (06: 502 contract, sidecar pid discovery).
- The `oracle/` protocol laboratory as a gate.
- The wire-tap idea (interview 0008 already said no).
- A separate Rust sibling (README bullet 7) — out of scope until Go ships.

## Recipes

- **Run the example app's suite against a target** (pattern, not the junkyard script):
  start the server at the built origin, then
  `BASE_URL=http://127.0.0.1:4173 mise x -- node node_modules/@playwright/test/cli.js test`.
- **Single-flight assertion that doesn't touch the wire**: count programmatic requests
  during the click (Playwright `page.on('request')` filtered to non-document, non-asset) and
  assert exactly one; assert the DOM updated; assert no `document` navigation.
- **Nonce-based static proof**: drop a random file into `static/` before `skgo build`, then
  fetch it from the Go binary and compare bytes; also fetch a hashed `_app/immutable/*` path
  from the built manifest and assert `cache-control: public, immutable, max-age=31536000`
  and etag/304 behaviour.
- **Colocated Go remote module**: `src/routes/messages/messages.remote.go` with
  `remote.Query`-wrapped functions; generator emits `src/routes/messages/messages.remote.ts`
  beside it; `src/routes/go.mod` isolates bracketed dirs; `skgo test ./...` includes linked
  packages.
