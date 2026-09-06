# Sprint 003: Bindings generation and server loads, CSR

Closes #4, #2, #3 (PR A+B) and #5 (PR C). Two worktrees, three Opus builders. This document
de-risks and draws seams; the builders choose packages, names, and commands. Two critique
rounds applied (2026-09-06); open decisions are marked **DECISION**.

## Pyramid Index

- L0: `skgo` generates kit's `.remote.ts` stubs and the Go registrar from colocated
  `*.remote.go` files (the brief's authoring model); handlers get a per-call event (cookies,
  headers, refresh, depends); Go answers `+page.server.ts`/`+layout.server.ts` loads over
  `__data.json` — proven against a production build with Vite stopped, and again behind
  `vp dev`.
- L1:
  - Generator (Track A): implement build plan §3 — source scan of `*.remote.go`, link tree,
    emitted `.remote.ts` + registrar. Delete the hand-written stub. Generated types must
    typecheck; `--check` mode; startup refuses hash/route drift.
  - Event + codec (Track B): one per-call handle for cookies/headers/refresh/depends; typed
    `Refresh` for argument-taking queries; #2/#3 fixed against kit's own `shared.spec.js`
    cases.
  - Loads (Track C): `__data.json` dispatcher with declared `uses`; layout chain is the goal,
    page-only the fallback.
- L2: `ephemeral/plans/2026-09-05-skgo-build-plan.md` §3; `ephemeral/brief/`;
  `drafts/SPRINT-003-research-bindings.md`, `drafts/SPRINT-003-research-data-json.md`;
  issues #2–#7.

## Facts that de-risk this (verified against pinned kit next.25)

Bindings (`drafts/SPRINT-003-research-bindings.md`):
- Kit needs only: the file exists at `vp build`, is imported by app code, and every runtime
  export is a top-level `query`/`command`/`query.live` wrapper with `'unchecked'`. Bodies are
  discarded on the client. Exported helpers break the build; type-only exports are fine.
- Dev picks up regenerated files (add/unlink → full reload; edit → HMR). No restart.
- **The authoring model is already decided** (brief `2026-09-05-skgo-vision.md:40,81-85,173-181`;
  build plan §3): colocated `*.remote.go` under `src/lib/` and `src/routes/**`,
  `//go:generate skgo …`, a `go/types` scan of exports, a link tree so bracketed route dirs
  are importable (junkyard technique, validated), emitted `.remote.ts` beside each source and
  a generated Go registrar. Sprint 002's `example/remotes.go` registry was a stand-in. Do not
  design a reflect-over-registry generator; `remote.go` needs no type retention.
- Kit's built manifest carries the remote hash set (`generate_manifest/index.js:117-118`).
  The adapter can record it, plus route ids/params and the `@sveltejs/kit` package version,
  in `skgo.manifest.json`; Go refuses to start when a registration's hash or route id is
  absent or the kit version is unsupported. This replaces "test that the path hashes to
  `worolc`" and turns the silent-404 class into a boot error in both modes.
- Remote functions are experimental "subject to change without notice"
  (`docs/20-core-concepts/60-remote-functions.md:14`); `__data.json` has been stable since
  kit 1. The version gate above is the mitigation.
- **polytype is a code generator, not a library, and that is the intended use** (Tyler,
  2026-09-06). It emits both sides of the wire: Go codecs (`MarshalJSON`/`UnmarshalJSON` on
  registered types, README:408-412) and TypeScript declarations
  (`//go:generate go tool polytype --typescript web/src/generated`, README:436), from one
  portable grammar (`internal/typegrammar/grammar.go`) that deliberately excludes
  bare-pointer-as-null, `omitempty`-as-optional, byte slices, maps and `any`. So skgo does
  **not** write a TS emitter and does **not** "mirror `encoding/json`": remote `In`/`Out`
  types are polytype-registered types, polytype generates their codecs and `types.ts`, and
  skgo's `.remote.ts` stubs import those names. Whatever polytype cannot project is rejected
  at generation — the admission rule comes for free. If polytype lacks something skgo needs
  (e.g. a way for skgo's scan to register the In/Out set, per-package `types.ts` for
  colocated route packages, codec generation for a type that has no marker), that is
  polytype work in `/Users/tyler/src/…/polytype`, not a workaround in skgo. Pin
  `@v1.0.0-rc.9`; `@latest` is v0.11.3.
- Server-pushed `q` entries are applied by the client without it having asked
  (`shared.svelte.js:139-150`), but a key that does not match the client's own
  `stringify_query_arg` bytes is silently dropped. `r: true` is read only by `form`.
- `invalidateAll` is deprecated in kit 3 (`client.js:2760`, `docs/20-load.md:643`) in favor
  of `refreshAll`, which reruns loads *and* active queries.
- The stub-throws proof inverts under SSR (#7): kit would execute stub bodies in-process.
  Stub bodies must come from one template seam so the SSR fork is a template swap.

Loads (`drafts/SPRINT-003-research-data-json.md`):
- Cold SPA boot fetches `<path>/__data.json`. Query params `x-sveltekit-invalidated`
  (bitstring over `[...layouts, leaf]`) and `x-sveltekit-trailing-slash`; no request headers.
- Response is NDJSON with a trailing `\n`: `{"type":"data","nodes":[…]}`; node
  `{"type":"data","data":<devalue flat>,"uses":{…}}`, same reducers as remotes; `null` for
  positions without a server load; `{"type":"skip"}` for bit `0` (mandatory once layouts
  exist). Redirect is HTTP 200 `{"type":"redirect"}`. Top-level error is `App.Error` JSON at
  the status with `application/json`. Omit `x-sveltekit-version` or echo kit's.
- The leaf is always the last position (`parse.js:23-31`). Page-only loads need no node
  info. **Layout loads need the route's layout chain** — kit's own CSR fixture is a layout
  server load (`kit/test/apps/no-ssr/src/routes/+layout.server.js`), because the load-only
  capabilities in a CSR app are layout-level: gate a subtree, redirect an unauthenticated
  deep link, `page.data` inherited by children. A page-level load is a `query` with worse
  granularity. Track C's first research question: cheapest source of the layout chain
  (the generated client `app.js` dictionary, or importing the server manifest — its node
  imports are lazy — or the generator, which knows every `layout.server.go`).
- **`uses` is load-bearing** (`client.js:1332-1361`): omitted → a param change never
  refetches, silently. `refreshAll` bypasses `uses`, so it proves nothing about them. Declare
  `uses` statically at registration (`UsesParams(...)`, `Depends(...)`), not by observing
  handler reads — Go cannot proxy-track reliably — and validate declared params against the
  route's actual params at startup.
- The client only asks for nodes kit compiled as having a server load: throwing
  `+page.server.ts`/`+layout.server.ts` stubs must exist at build. In prod no server JS
  ships, so "the stub throws" is observable only under `vp dev`.
- Kit's `[^]` is JS-only regex; RE2 rejects it. `static.go:104-111` compiles patterns without
  the rewrite today — a `[...rest]` route would fail at startup.
- `vp dev` answers `__data.json` itself → Go intercepts before proxying, only for routes with
  a registered Go load; everything else passes through (dev → kit, prod → 404 empty body).

## Seams

**A — generator.** Build plan §3 as written: `skgo` command (`go/types` scan of `*.remote.go`
and `page.server.go`/`layout.server.go` exports, link tree, emitted `.remote.ts` and load
stubs via one body-template seam, generated registrar), `--check` mode, the polytype
hand-off (skgo's scan registers every In/Out type with polytype; polytype's `types.ts` is
what the stubs import), and the `skgo.manifest.json` hash/route/version emission in
`example/web/skgo-adapter.js`. Restructures the example to the brief's layout
(`example/remotes.go` → colocated `*.remote.go`); owns the generated files and the
`go:generate` gesture. Does not edit `remote.go`, the codec packages, `static.go`, or
`.svelte` files. Wires its registrar into `example/cmd/main.go` in one region; C owns the
other; whoever merges second rebases.

**B — event + codec.** Owns `remote.go`, `remote_live.go`, `internal/remotearg`,
`internal/devalue`. Delivers: a per-call event reachable from the handler (request, cookies,
response headers, `Refresh(query, arg)`, `Depends`) without changing the
`func(ctx, In) (Out, error)` shape; startup refusal when a registration's hash is absent from
`skgo.manifest.json`; #2/#3 fixed with kit's `runtime/shared.spec.js` `stringify_remote_arg`
/`parse_remote_arg` cases ported as goldens plus the astral/private-use pair; command-arg
stringify refuses maps. Owns `example/web/src/lib/*.svelte` and `remote.feature` for the
refresh scenario. Small; lands first so A's registrar targets the final API.

**C — loads.** Owns `static.go` (one matcher, `[^]` rewrite, route table from
`skgo.manifest.json`), the `__data.json` dispatcher, `PageLoad`/`LayoutLoad` bindings with
declared `uses`, `example/cmd/main.go` load wiring, a **new route subtree** (not `/todos`)
with hand-written throwing stubs until A's generator emits them, `loads.feature`, and a
`__data.json` fixture counter. Layout chain first; page-only is the shrink fallback.

## Functional Definition of Done

PR A+B (closes #4, #2, #3):
1. No hand-written `.remote.ts` remains; `go generate` reproduces them from colocated
   `*.remote.go`; `vp build` + the existing Playwright suite pass unchanged; `skgo --check`
   fails on a stale stub; the binary refuses to start if a registered hash is missing from
   the manifest or the kit version is unsupported.
2. The stubs import polytype-generated types; the TypeScript typechecks (`vp check` or
   `tsgo --noEmit` over `example/web`, run from `go test`), and a deliberately wrong Go type
   makes it fail. A Go value round-trips through the polytype codec into a shape the
   generated TS type admits (one golden).
3. A Go command invoked **without** `.updates()` on the client mutates a todo and the open
   `/todos/[id]` page shows the new text from that one round trip (`remotes` counter proves
   no second request). A command sets a cookie the browser then sends back.
4. #2 and #3 have failing-then-passing tests; `go vet`, `go test` green; dev mode still
   answers remote calls.

PR C (closes #5):
5. Production binary, Vite stopped: a deep link into the new subtree renders layout data and
   page data that only Go has, on cold boot.
6. Navigation between two params refetches the page node only (`uses.params`; layout comes
   back `skip`); `refreshAll()` reruns loads and the active query in one gesture.
7. A layout load redirects an unauthenticated deep link (`{"type":"redirect"}` at 200) and
   an error envelope renders `+error.svelte`; both also unit-tested against captured shapes.
8. `vp dev` behind the Go binary: the subtree works with the JS stubs throwing; a route with
   no Go load still reaches kit's dev server.
9. Adapter refuses a Go-loaded route that is also prerendered.

## Shrink order if the session blows out

Layout loads (fall back to page-only; DoD 6–7 shrink accordingly) → the cookie half of DoD 3
→ the astral corpus in #2 (keep the ported `shared.spec.js` goldens and #3).

## Advice and traps

- Layout server loads, streaming promises, `form`, `query.batch`, `prerender`, SSR: out
  except as stated above.
- Never emit an exported helper from a stub; keep `unimplemented` module-private.
- `example/web/node_modules` must be installed and `vp build` run before `go vet ./example/...`
  passes (`//go:embed all:build`).
- Three `vp` binaries exist; only the project-local one works (`example/mise.toml` `_.path`).
- Token cache: `ephemeral/inspiration/` is a directory of symlinks, gitignored. In a worktree,
  symlink it to the root's copy and **never `git add` it**.
- polytype: pin `@v1.0.0-rc.9` explicitly; `@latest` is v0.11.3.
