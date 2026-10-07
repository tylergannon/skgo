# Adversarial review — RequestEvent plan, round 03

**Target:** `ephemeral/plans/20261006-request-event.md` (untracked, worktree `bad1`, baseline
`916ccdc`), revised after round 02.
**Kind:** design plan; implementation deferred until #262 lands. No code under review.
**Operating constraints honoured:** read-only except this artifact. No caller narrowing detected.

## Evidence inspected

Everything from rounds 01–02, re-read where the plan changed, plus for this round:
`prerender_service.go` (whole), `prerender_load.go:1–140`, `internal/gen/prerender_command.go`,
`internal/adapter/skgo-adapter.js:95–115` (per-request `emulate().platform()`),
`internal/adapter/skgo-adapter/prerender.js:560–740` (callback POST shape), Kit
`src/runtime/server/respond.js:420–450` (platform emulation and the internal-only
`before_handle`), `src/types/internal.d.ts:199,713`, `src/exports/public.d.ts:424–430`
(`Emulator` exposes only `platform`), `src/core/postbuild/prerender.js:380–520`,
`src/exports/vite/dev/index.js:443`; `internal/newapp/gofiles/server.go.tmpl:2,18–96` and
`cmd/main.go.tmpl:2` (scaffold package layout); `internal/gen/scan.go:147–179,352` (what the
generator discovers today); `example/web/src/` listing (no `hooks.server.ts` exists).

## Round-02 finding — status

Entry-point coverage → **resolved in intent**: §2 now enumerates main handler,
`FetchConfig.Handler`, `SSROptions.Fetch` and the prerender command, requires the starter to wire
both fetch paths, and names a generated `RequestBoundary`. The prerender row, however, introduced
new material that this round's findings are about. Nitpicks from round 02 (address inheritance
across the valueless context, `makeParams` error shape, typed-mismatch panic) are incorporated.

## Findings

### 1. Issue — hook selection is unspecified and §1's placement advice contradicts §2's import graph

§2 says generated server bindings expose `RequestBoundary(cfg, next)` which "supplies the selected
hook", and the generated prerender command "supplies the same generated `RequestBoundary`". The
signature has no hook argument, so the generator must *discover* the application's hook and compile
it into both binaries. Nothing in the plan says how: no marker, no signature rule, no scan
location. Today the scanner discovers transport entries from `src/hooks.go` (`scan.go:147–179,
352`) and nothing hook-shaped. "Selected" is used four times without a definition.

The import diagram now has `Wiring → Hooks`. §1 still says to "keep [the hook] in server assembly
or a separate hooks package". Server assembly imports the generated bindings — the scaffold's
`server.go.tmpl:18–96` (`package app`, module root) calls `generated.Actions()`, `.Loads()`,
`.Remotes()`, `.Endpoints()` — so a hook there gives `generated → app → generated`, an import
cycle, and the prerender command (`main_gen.go:13`) could never link it. Only the "separate hooks
package" branch is viable, and it must be one that imports neither server assembly nor generated
bindings.

Impact: the single mechanism that makes "a generated typed application must install this
boundary" true is undefined, and the one placement the plan recommends first cannot compile.
Fix: define the hook marker/signature the generator recognises (and the diagnostic when there are
two, or none), state the package constraints positively (importable by generated bindings; imports
only `params`, `app`, domain packages), and drop "server assembly" from §1. Also note the name
clash: the scaffold's root package is already `app` (`server.go.tmpl:2`) and the plan adds
`internal/app` with the same name.

### 2. Issue — the prerender logical-request design is neither specified enough to build nor scoped as a follow-up

§2 "Prerender is in scope" commits to running the Go `handle` once per Kit prerender request with
"the selected Go middleware's resolve surround[ing] all callbacks … with before/after execution
once, existing header/cookie/redirect/error semantics". Checked against the pin and the current
bridge, the following are undefined, and each decides feasibility:

- **What `resolve` returns and when.** `Resolve` returns `*http.Response` (`handle.go:66`). In the
  build, the document is produced by Node after an unknown number of callback POSTs
  (`prerender_service.go:96–160`, one POST per load/remote). The plan does not say what response
  the Go hook's `resolve` yields, nor whether `ResolveOptions` apply: Kit applies
  `transformPageChunk`, `filterSerializedResponseHeaders` and `preload` to prerendered output
  (`respond.js` passes the same `resolve`), and the example's `SerializedHeaders` middleware
  relies on the filter. Either the HTML and hydration data cross the bridge both ways, or those
  options silently do nothing at build — a deviation the plan must state.
- **Who owns the lifecycle end.** Kit's public `Emulator` has only `platform()`
  (`public.d.ts:424–430`), created per request (`respond.js:425–442`); `before_handle` is internal
  and used only by the dev server (`internal.d.ts:199`, `vite/dev/index.js:443`). So the only
  public end-of-request point is a `handle` in `src/hooks.server.ts`. The plan says "a generated
  Kit build hook can forward lifecycle operations" — "can", with no statement that skgo now owns
  `src/hooks.server.{ts,js}` generation, how it stays inert outside `building`, or what happens
  when the app already has one (the example has none; `example/web/src/` listing).
- **Refusal ordering.** Kit runs `handle` before any load; a hook redirect/error at prerender is
  written by the prerenderer. The plan's DoD requires "refusal propagation" but §2 does not say
  that begin-request must complete (and return the refusal to Node) before Node resolves, nor how
  a Go `*Redirect`/`*HTTPError` is re-thrown as Kit's own `redirect()`/`error()` in the generated
  hook.

This is the largest new subsystem in the plan — a cross-process request registry with begin/
callback/end, cancellation and concurrency, a new generated JS hook, `PrerenderServiceOptions`,
and route-metadata transport — introduced to make `handle` run at build, which the user did not
ask for and the product has never done. CLAUDE.md asks for missions with explicit acceptance, not
a design that admits its central contract ("resolve surrounds all callbacks") is unresolved.
Fix: one of two. (a) Specify the three points above as public contracts, give the prerender
lifecycle its own mission and acceptance in §6, and accept that it gates the feature. (b) Keep
this plan to the nil-hook binding at prerender (empty locals, `ClientAddress` error, per-callback
binding, `/inputs` requestless) and record "the Go `handle` does not run during prerender" as a
mapped deviation beside platform/tracing, with the full lifecycle as a linked follow-up. Either is
acceptable; the current text is neither.

### Nitpicks

- §2 prerender: "retain its locals for the complete request … releasing request state only after
  callbacks and deferred work finish" — the service already tracks per-callback cancellation
  (`prerender_service.go:120–145`); say whether deferred/streamed chunks (`prerender_load.go`
  `promises.settled`) extend the logical request or are cut at Kit's response.
- DoD "Prerender has real request ownership" asserts "before/after hook ordering"; until finding 2
  is settled there is nothing observable "after".
- §5 "The configured locals package must already exist" — for the generated test fixtures under
  `internal/gen/testdata` this means every fixture grows an `app` package; say so in §6.4 so the
  fixture count is part of the deliverable rather than discovered.

## Outcome

`material findings remain`
