# skgo build plan — research findings and proposed order

Source brief: `ephemeral/brief/2026-09-05-skgo-vision.md`. Semantic index over the
junkyard and pinned upstream sources: `ephemeral/semantic-index/README.md`.
Everything below cites index leaves; open the leaf for the upstream line numbers.

## 0. What the research changed

1. **Kit's remote ids are computable in Go.** `id = hash(file) + "/" + export`,
   where `file` is the vite-root-relative path of the `.remote.ts` module (extension
   included) and `hash` is djb2 over UTF-16 code units iterated from the end,
   rendered base36. Same in dev and build. The junkyard's whole "capture ids from a
   Vite plugin during `vp build`" machinery is unnecessary. → `sources/kit/remote-server/ids-and-build.md`.
2. **The wire protocol is fully source-derivable.** Envelopes, base64url devalue
   payloads with kit's reducers (`__skrao`/`__skram`/`__skras`/`__skraf`), the
   SSE framing for `query.live`, the binary `application/x-sveltekit-formdata`
   layout for `form`, the `__data.json` node array and ndjson deferral — all in
   pinned source with unit tests we can port. → `sources/kit/remote-server/*`,
   `sources/kit/server-runtime/*`, `sources/kit/portable-tests/*`.
3. **Polytype rc.5 emits TypeScript** (`--typescript DIR`), so skgo does not need
   its own Go→TS projection. rc.6 (fluent `Declare` API) is imminent; target it.
   → `sources/libs/polytype.md`.
4. **The junkyard's colocation + link-tree technique works** and was validated with
   `go build`/`go vet`/gopls on Go 1.27.1 macOS arm64. Keep it. → `sources/junkyard/colocation/link-tree.md`.
5. **12 of the junkyard's 23 Playwright tasks port unchanged** to a CSR-only Go
   server; 8 need one-line edits; 3 are SSR-only and get dropped. → `sources/junkyard/app/csr-portability.md`.
6. **AGENTS.md in this repo describes the old design** (adapter + Node sidecar +
   SSR). It must be rewritten to the brief before anyone builds against it.

## 1. Repository shape (from the brief, refined)

```
skgo/                         go.mod github.com/tylergannon/skgo   (library + CLI)
  cmd/skgo/                   the generator/CLI (gen-bindings, new, dev helpers)
  internal/devalue/           port of devalue stringify/parse (+ ported tests)
  internal/remote/            remote-function protocol: envelopes, payload codec, SSE, formdata
  internal/data/              __data.json responder: nodes, invalidation bitmask, ndjson deferral
  internal/routing/           route tree from filesystem, id sorting, param matching (port of create_manifest_data + utils/routing)
  internal/static/            sirv-equivalent static serving + mrmime table
  internal/proxy/             dev reverse proxy to vp dev (HTTP + WebSocket HMR)
  internal/gen/               AST scan of markers, link tree, emit Go + TS
  skgo.go                     public marker API + Server/Handler types
example/                      go.mod github.com/tylergannon/skgo/example
  cmd/main.go                 flag parsing; skgo.Serve(...)
  businesslogic/
  generated/                  //go:generate skgo gen-bindings ...; bindings.go; links/<enc>/ symlinks
  web/                        vite root: package.json, vite.config.ts, src/
    src/lib/*.remote.go       + generated *.remote.ts
    src/routes/go.mod         module boundary so `./...` never enters bracketed dirs
    src/routes/**/{data.remote.go, page.server.go, layout.server.go}
                              + generated {data.remote.ts, +page.server.ts, +layout.server.ts}
```

Trap: Go ignores source files whose names start with `_` or `.`. The brief's
`_data.remote.go` for generated output would be silently dropped by `go build`.
Use `data.remote_gen.go` / `skgo_gen.go` instead.

## 2. Build order

### Phase A — bare server, no codegen (single worktree)
Deliverable: `example/cmd` binary that (a) in prod serves `web/build` (or
`.svelte-kit/output/client` + prerendered) with sirv-equivalent rules and the SPA
fallback; (b) in dev proxies everything to `vp dev` incl. the HMR websocket; (c)
mounts a hand-written Go handler at one remote URL and one `__data.json` URL,
computed with the Go port of kit's hash, called from a hand-written throwing
`.remote.ts` stub; (d) one Playwright test proves Go answered. Air config for
rebuild. This is where `internal/devalue`, `internal/remote`, `internal/data`,
`internal/routing`, `internal/static`, `internal/proxy` get built by direct port
with ported tests. See §4 for the port list and order.

### Phase B — fan out (two worktrees)
- **codegen**: `skgo gen-bindings` (§3). Gate: example app's remote functions and
  page/layout loads are all generated; every `.remote.ts`/`+page.server.ts` in
  `web/src` is generated and throws; Playwright suite (ported from the junkyard,
  §5) green against the Go binary.
- **project generator**: `skgo new` emits the example shape with the pinned
  toolchain (kit 3.0.0-next.25, vite-plus 0.3.0, TS 6.0.3, mise, Air config,
  playwright), `vp install` works, `skgo gen-bindings` runs via `go generate`.

### Phase C — protocol breadth
`query.live` (SSE), `form` remotes (binary formdata; JS-enabled path only),
`query.batch`, `prerender` remotes (build-time problem, see §6), classic
enhanced form actions, streamed loads (ndjson), transport hook types.

## 3. Bindings generation — concrete design

The generator is one Go program, `skgo gen-bindings --src-root <example/> --web <example/web>`,
invoked from `example/generated/config.go` via `//go:generate`. It runs in five steps.
Every step names the upstream code it mirrors.

### 3.1 Scan the route tree (mirror of kit's `create_manifest_data`)
Walk `<web>/src/routes` and `<web>/src/lib`. Collect:
- remote files: `*.remote.go` (in `lib/` or any route dir);
- load files: `page.server.go`, `layout.server.go` (route dirs only).
Build the same route tree kit builds from `+page*`/`+layout*` files: route id, segments,
layout chain, sort order (port `core/sync/create_manifest_data/index.js` + `sort.js`,
tests in its `index.spec.js`; see `sources/kit/server-runtime/routing.md`). Go's tree and
kit's tree are built from the *same directories*, so node indexes in `__data.json` line up
as long as Go emits a `+layout.server.ts` / `+page.server.ts` stub wherever it has a
`layout.server.go` / `page.server.go` (the client requests `__data.json` only for nodes
that have a server module; `sources/kit/build-adapt/spa-and-prerender.md`).

### 3.2 Make every Go package importable (the link tree; junkyard technique, kept)
Route directories contain `[id]`, `(group)`, `[...rest]`, which are illegal in Go import
paths, and `web/src/routes/go.mod` is a module boundary so `go build ./...` from `example/`
never descends into them (`sources/junkyard/colocation/link-tree.md`).
For each directory that holds Go files:
- subdirectory of `routes/`: create `example/generated/links/<enc>` as a **directory symlink**
  to the authored dir (`enc` = lowercase unpadded base32 of the `web`-relative path — injective,
  deterministic, validated with go build/vet/gopls on Go 1.27 macOS arm64);
- the root `routes/` dir itself: create `example/generated/links/<enc>/` as a real directory
  containing **per-file symlinks** to each `*.go` (never `go.mod`), as the brief specifies;
- `lib/` dirs: they are ordinary packages of the `example` module (`…/example/web/src/lib`);
  no link needed unless the dir name is illegal.
Record `links.json` (link → target) and treat the whole `links/` dir as disposable output.
Packages are then loaded with `golang.org/x/tools/go/packages`
(`NeedTypes|NeedSyntax|NeedTypesInfo`) via the link import paths.

### 3.3 Find the markers (mirror of the junkyard scanner, adapted to marker functions)
The public API in module `github.com/tylergannon/skgo`:
```go
type Marker struct{}
func Query[In, Out any](fn func(context.Context, In) (Out, error)) Marker
func Command[In, Out any](fn func(context.Context, In) (Out, error)) Marker
func Form[In, Out any](fn func(context.Context, In) (Out, error)) Marker        // Phase C
func LiveQuery[In, Out any](fn func(context.Context, In) (<-chan Out, error)) Marker  // Phase C
func Prerender[In, Out any](fn func(context.Context, In) (Out, error), opts ...PrerenderOpt) Marker // Phase C
func PageLoad[Out any](fn func(context.Context, *LoadEvent) (Out, error)) Marker
func LayoutLoad[Out any](fn func(context.Context, *LoadEvent) (Out, error)) Marker
```
Authored usage: `var _ = skgo.Query(getUser)`. The scanner finds `var _ = <call>` where the
callee resolves — by `TypesInfo.Uses[ident].Pkg().Path() == "github.com/tylergannon/skgo"`,
never by source text — to one of those names, takes `call.Args[0]`, resolves it to a
`*types.Func` in the same package, and reads `In`/`Out` from its `*types.Signature`
(the generic instantiation gives them for free). The generic signatures make a wrong
function shape a compile error at authoring time, before generation runs.
Exported name in TS = the Go identifier (unexported Go names are fine: the generated Go
lives in the same package). Reject: functions from another package, duplicate names,
`In` that is not JSON-representable per polytype's rules.

### 3.4 Types (polytype) — where the honest gap is
For each `In`/`Out` type that is a named type, skgo needs (a) `ValidateJSON`+codec on the Go
side and (b) a TypeScript declaration to import in the stub.
- Scalars, slices/maps of scalars: mapped directly (string→string, ints/floats→number,
  bool, []T→T[], map[string]T→Record<string,T>); no polytype involvement.
- Named struct types **defined in the same package as the remote function**: skgo emits
  `skgo_schema.go` (`//go:build jsonschema`) with `NewJSONSchemaMethod(T.Schema)` (rc.5) /
  `Declare(T.Schema)` (rc.6) registrations, then runs
  `go tool gen-jsonschema --validate --typescript <web>/src/lib/skgo/types/<enc-pkg>` on
  that package, which writes `jsonschema_gen.go` + `jsonschema/` next to the source and
  `types.ts` under `web/src/lib/skgo/types/<enc-pkg>/`. This is exactly what the junkyard
  did and validated (`sources/junkyard/remote-codegen/typeglue.md`).
- Named types **defined in another package** (the brief's `businesslogic.UserProfile`):
  polytype registers types by a method on the type, which can only be declared in the
  type's own package. **I do not know whether rc.5/rc.6 can register a top-level schema for
  a foreign type from the route package** (`NewJSONSchemaFunc[T]` takes a free function
  "with the same receiver type" — the docs do not say whether T may be foreign). Two
  fallbacks that are known to work today, in preference order:
  1. treat `businesslogic` as a polytype package in its own right: the developer (or
     `skgo new`) adds the `//go:generate go tool gen-jsonschema --typescript …` directive
     there, and skgo's generator only *locates* the resulting `types.ts` by package path
     and imports from it. This is the junkyard's "developer-owned registration" rule and
     it keeps schemas next to the types they describe.
  2. if a foreign type has no registration, fail generation with a message naming the
     package and the directive to add. No silent inference.
  Decision needed from Tyler once rc.6 ships: whether polytype will support foreign-type
  registration directly (which would make option 1 automatic).

### 3.5 Emit
Per package with markers, `skgo_gen.go` (same package; **not** `_data.remote.go` — Go
ignores `_`-prefixed files):
```go
// Code generated by skgo. DO NOT EDIT.
package users_show
var SkgoBindings = []skgo.Binding{
    skgo.QueryBinding("<hash>/getUser", getUser),   // hash computed in Go (§0.1)
    skgo.PageLoadBinding("/users/[id]", load),
}
```
`skgo.QueryBinding` is a generic helper in the runtime that wraps `fn` into an
`http.Handler` doing: decode `payload` (base64url → devalue → JSON → `ValidateJSON` →
`json.Unmarshal` into `In`), call `fn`, encode `Out` (JSON → devalue with kit's reducers +
transport hooks → envelope `{"type":"result","data":…}`), map `skgo.Redirect`/`skgo.HTTPError`
to kit's envelopes (`sources/kit/remote-server/request-handling.md`). Command bindings also
run the `refreshes` allow-list intersection. Load bindings produce one node for
`__data.json` (`sources/kit/server-runtime/data-requests.md`).

Per remote file, the sibling `<name>.remote.ts`:
```ts
// Code generated by skgo. DO NOT EDIT.
import { query, command } from '$app/server';
import type { UserID, UserProfile } from '#lib/skgo/types/<enc-pkg>/types.js';
export type { UserID, UserProfile };
export const getUser = query('unchecked', (_id: UserID): UserProfile => {
  throw new Error('skgo: getUser is implemented in Go; this stub must never execute');
});
```
The top level must be a real `query(...)` call because kit *executes the module in Node*
(dev: SSR runner; build: `analyse`) to learn each export's kind (`sources/kit/remote-client/client-transform.md`).
Only the body throws. Per route dir with loads, `+page.server.ts` / `+layout.server.ts`:
```ts
export const load = (): never => { throw new Error('skgo: load is implemented in Go'); };
```
Kit never executes these in a CSR-only app served by Go (no SSR, no prerender), but their
*presence* makes the client request `__data.json` for that node.

`example/generated/bindings.go`:
```go
// Code generated by skgo. DO NOT EDIT.
package generated
import (
    l1 "github.com/tylergannon/skgo/example/generated/links/<enc1>"
    …
)
var Routes = skgo.RouteTree{ /* route id, pattern, layout chain, node indexes */ }
func RegisterHandlers(mux *http.ServeMux, opts skgo.Options) {
    r := skgo.NewRegistry(Routes, opts)     // base, appDir, origin, transport, body limit
    r.Add(l1.SkgoBindings...)
    r.Mount(mux)   // "/_app/remote/{hash}/{name}" and "GET <route>/__data.json" patterns
}
```
Remote URLs are mounted as `"/_app/remote/{id}/{name}"` with a map lookup; data URLs use
Go 1.22 mux patterns derived from the route id (`[id]`→`{id}`, `[...rest]`→`{rest...}`,
`[[opt]]`→two patterns, matchers → handler-level check, then kit's sort order for
ambiguity). Registry code is hand-written in `skgo`, so the generated file is small and
compile-stable with no build-order dependency on a Node step (the junkyard's Stage B
bootstrap trap is gone).

### 3.6 What I cannot yet claim
- The foreign-type registration question in §3.4.
- Whether `go/packages` resolves a package through a *directory* symlink identically on
  Linux (junkyard validated macOS only). Phase A includes a Linux CI job to settle it.
- `prerender` remotes: kit runs them in Node at build time for each `inputs` value; a
  throwing stub fails `vp build`. Options are (a) defer the feature, (b) let the stub
  fetch from a running Go server during build. Deferred to Phase C; not in the gate.
- Non-JS form fallback (`?/remote=` POST re-rendered by SSR) cannot exist without SSR;
  forms are JS-enabled only. Classic `+page.server.ts` actions work only via `use:enhance`.

## 4. Direct-port list (kit → Go), in order

Per Tyler's addendum: copy upstream behavior by reading the pinned source and port its
unit tests; run a Node app only when reading is not enough. Full table with difficulty
ratings: `sources/kit/portable-tests/port-map.md`. Order below is by protocol value and
dependency.

| # | Upstream (under `reference/`) | Go package | Notes |
|---|---|---|---|
| 1 | `devalue/src/{stringify,parse,base64,constants}.js` + `devalue/test/index.test.js` | `internal/devalue` | Everything else depends on it. Needs an `Undefined` sentinel, ordered objects, JS number formatting, devalue's own string escaper. Do not port `uneval` or `operations`. → `sources/libs/devalue.md`, `sources/kit/portable-tests/devalue-port.md` |
| 2 | `kit/.../runtime/shared.js` + `shared.spec.js` | `internal/remote/args` | base64url + `__skra*` reducers/revivers, sorted keys for GET kinds. |
| 3 | `kit/.../runtime/server/remote-functions.js` + `.spec.js` | `internal/remote` | `/_app/remote/*` handler, envelopes, live SSE, method/415/404 rules. Port the two spec tests plus the table tests the leaf lists. |
| 4 | `kit/.../runtime/server/{cookie,csrf}.js` + specs | `internal/cookie`, `internal/csrf` | Cookie path defaults; CSRF origin rules and exact error text. |
| 5 | `kit/.../utils/routing.js`, `utils/params.js`, `utils/url.js` + specs | `internal/routing` | Route-id regex, matchers, rest/optional, trailing slash. |
| 6 | `kit/.../core/sync/create_manifest_data/{index,sort}.js` + spec + 32 fixture dirs | `internal/manifest` | Copy fixtures into `testdata/`; port the 28-id sort golden literally. skgo's route scan must agree with kit's. |
| 7 | `kit/.../runtime/server/data/index.js`, `page/data_serializer.js`, client `parse.js`/`stream.js` + specs | `internal/data` | `__data.json` nodes, `uses`, ndjson chunks, `text/sveltekit-data`. |
| 8 | `kit/.../runtime/form-utils.js` + spec | `internal/form` | Binary `application/x-sveltekit-formdata`; ten exact 400 strings. Phase C. |
| 9 | `kit/.../runtime/server/page/actions.js` | `internal/actions` | Enhanced actions only (`x-sveltekit-action`), ActionResult devalue. |
| 10 | `sirv/packages/sirv/index.mjs` + `sirv/tests`, `mrmime/deno/mod.ts` | `internal/static` | Candidate order, weak ETag format, `Vary`, `.br/.gz`; generate the MIME table. Do not copy sirv's vacuous dotfile test. |
| 11 | `kit/.../utils/hash.js` | `internal/kithash` | 10 lines + golden values; done in Phase A step 1. |

Do not port: `uneval`, CSP, `serialize_data` (SSR inlining), `load_data` SSR paths,
static page-option analysis, devalue `operations`.

## 5. Acceptance suite

Playwright, pointed at whatever `BASE_URL` names, protocol-agnostic (DOM, navigation,
document reloads, count of programmatic requests, response headers). Launch line that
works under mise/vp: `mise x -- node node_modules/@playwright/test/cli.js test`
(`sources/junkyard/app/toolchain.md`).

Seed the suite from the junkyard guestbook (`sources/junkyard/app/e2e-suite.md`,
`csr-portability.md`):
- **12 tasks port unchanged**: single-flight command (exactly 1 fetch, 0 document loads),
  `query.live` push across browser contexts, multipart upload, layout+page load cookie,
  transport round-trip of a class instance, deep link vs client navigation, unknown-route
  404, redirect from load, enhanced form actions (login/logout/validation), static asset
  headers + ETag 304, brotli negotiation, 413 body limit.
- **8 need one- or two-line edits** for CSR-only (document status on load errors, request
  id compared against the data response, streaming, "0 data requests after hydration").
- **3 are SSR-only and are dropped**: both `javaScriptEnabled: false` tests and
  "home page is server-rendered".
Then borrow kit's own scenarios from `packages/kit/test/apps/async/src/routes/remote/*`
and `no-ssr` (`sources/kit/test-apps/remote-scenarios.md`, `no-ssr-guarantees.md`,
`load-and-action-scenarios.md`) for validation errors, redirects from queries, batch,
refresh cycles, and error hydration — each marked CSR-viable or not in the leaf. The
leaf's ordered top-10 (single-flight matrix, batch, live streaming, form basics,
10 MB upload, validation, private/non-exported queries, context guards, no-ssr suite,
action envelope + CSRF) is the Phase C backlog. Open question recorded there: whether a
native (non-enhanced) form POST result survives under `ssr=false` — no kit test shows it.

Gate per phase: Phase A = 1 remote + 1 load task green; Phase B codegen = the 12
unchanged tasks green with every stub generated; Phase C = the borrowed kit scenarios.

## 6. Known unknowns and honest gaps

1. **Foreign-type registration in polytype** (§3.4). Blocks "arbitrary function" until
   rc.6 semantics are known. Fallback is defined and known to work.
2. **Linux + directory symlinks with `go/packages`**: unverified. Phase A adds a Linux CI job.
3. **`prerender` remotes**: kit executes them in Node at build; a throwing stub fails the
   build. Deferred; not in any gate until a design (build-time fetch from Go, or opt-out)
   is chosen with Tyler.
4. **Non-JS fallbacks**: impossible without SSR (classic actions without `use:enhance`,
   `form` remote `?/remote=`). Documented limitation, not a bug.
5. **`vp dev` HMR websocket forwarding**: Vite behavior, not in pinned kit source; verify
   against pinned vite-plus 0.3.0 when building `internal/proxy`.
6. **Byte-equality vs structural equality for devalue output**: JS dedups equal primitives
   and orders integer-like keys first; decide in Phase A whether Go promises byte-equal
   output (ordered object type) or only client-acceptable output. Recommendation: byte-equal,
   because `query.live` dedupes on the serialized string.
7. **`x-sveltekit-version`**: must equal the client's baked `version.name` or be omitted;
   the thin adapter should emit it into the manifest so Go can match.
8. **Origin**: kit fixes `paths.origin` at build; Go must listen at that origin in every
   environment or every POST 403s. The `skgo new` template must make this explicit.
9. **AGENTS.md** still describes the old design; proposed replacement at
   `ephemeral/plans/AGENTS.md.proposed` awaiting Tyler's approval.
