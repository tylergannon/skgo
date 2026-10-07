# Adversarial review — RequestEvent plan, round 04

**Target:** `ephemeral/plans/20261006-request-event.md` (untracked, worktree `bad1`, baseline
`916ccdc`), revised after round 03.
**Kind:** design plan; implementation deferred until #262 lands. No code under review.
**Operating constraints honoured:** read-only except this artifact. No caller narrowing detected.

## Evidence inspected

Everything from rounds 01–03, re-read where the plan changed, plus for this round: Kit
`types/index.d.ts:1385–1416` (`ResolveOptions` signatures), `src/runtime/server/page/render.js:
322,333,373,612–626` (where and how each option is invoked), `src/runtime/server/page/
serialize_data.js:38–52` (`filter` invocation), `src/runtime/server/respond.js:386–462`
(platform assignment precedes `handle`), `src/core/postbuild/prerender.js:380–441` (prerenderer
consumes the response with `arrayBuffer()`); skgo `internal/adapter/skgo-adapter/entry.js:27–39`
(the goja bundle loads only universal hooks), `internal/adapter/skgo-adapter/prerender.js:
321–360,632–690` (owner lifecycle, callback payload), `internal/newapp/newapp.go:436–452`
(scaffold's hooks-server classification).

## Round-03 findings — status

1. Hook selection / placement → **resolved**: `HookPackage`/`HookSymbol` with validation rules,
   scaffolded `internal/serverhooks/handle.go`, server assembly excluded, `app` name clash noted.
2. Prerender lifecycle → the plan chose to specify fully and gate the feature on it (§2 contracts
   1–5, mission §6.4, DoD row). Ownership of `src/hooks.server.*`, begin-before-resolve with
   refusal re-throw, resolve-as-real-response, and body-wrapper completion are now explicit and
   consistent with the pin's public surface (`Emulator.platform` only; platform assigned before
   `handle` at `respond.js:425–446`). Nitpicks (deferred chunks, fixture `app` packages) are
   incorporated. One of the new contracts is not satisfiable as written — finding 1 below.

## Findings

### 1. Issue — contract 4 requires synchronous Node→Go calls that the pin's API does not allow

§2 contract 4: "Kit invokes proxies for `transformPageChunk`, `filterSerializedResponseHeaders`,
and `preload` at its ordinary render/serialization points; each proxy calls the retained Go
function on that logical request and returns its result … no option may silently become a no-op
during a build." The DoD row then requires "fetch-header serialization and preload options" to be
demonstrated, and §2 makes the lifecycle "a feature gate".

The pin types two of the three as synchronous booleans and calls them synchronously:

- `filterSerializedResponseHeaders?: (name, value) => boolean` (`types/index.d.ts:1399`), called
  as `if (filter(key, value))` inside `serialize_data` (`serialize_data.js:52`).
- `preload?: (input) => boolean` (`types/index.d.ts:1411–1415`), called as
  `if (resolve_opts.preload({...}))` three times in `render.js:322,333,373`.
- Only `transformPageChunk` is `MaybePromise` and awaited (`types/index.d.ts:1392`,
  `render.js:618`).

A Node proxy cannot await a Go round-trip inside a synchronous boolean callback. The only ways to
honour "calls the retained Go function and returns its result" are a blocking bridge
(`Atomics.wait` on a worker-backed channel, or a synchronous child-process call per invocation),
which the plan neither names nor costs, or pre-answering, which is impossible for
`filterSerializedResponseHeaders` (inputs are the headers of responses fetched during the
render). As written, an implementer reaches contract 4, finds the pin forbids it, and — by the
plan's own rule — "return[s] to design", so the gate fails on the first attempt.

Fix: decide now, in the plan. Either (a) require a synchronous bridge for the two boolean
options and state the mechanism and its acceptance (a build with a Go `preload` that rejects one
asset and a Go filter that admits one header, both observed in the prerendered output), or (b)
proxy only `transformPageChunk` and declare the two synchronous options unsupported at build —
a diagnostic at generation time when the selected hook passes either option while prerendering
is enabled, recorded beside platform/tracing as a mapped deviation. "No option may silently become
a no-op" is satisfiable under (b) only by that diagnostic, so say so.

### 2. Issue — contract 3 prescribes a duplex no-buffering stream the consumer never exercises

§2 contract 3: "Body consumption must proceed concurrently across the bridge so neither side
requires the other to buffer a complete streamed document before making progress", with
cancellation, body errors and Close propagating both ways. Kit's prerenderer reads the response
with `Buffer.from(await response.arrayBuffer())` (`prerender.js:441`) before doing anything with
it, so at build there is no observer of chunk-level progress; the requirement adds a backpressure
protocol across two processes for a property no test can distinguish from "send the body when it
is complete". CLAUDE.md: "Delegate missions, not methods". The observable contracts — the Go hook
sees the real status/headers/body, its after-logic is applied, completion/cancellation release
state — are already stated in contracts 3 and 5. Fix: strike the no-buffering sentence; keep
"the Go hook's `resolve` yields the actual Kit response and the prerenderer receives the Go-
selected response".

### Nitpicks

- §2 contract 2 says the Go hook "must either reach its final resolve or answer/refuse" before
  Node resolves. With contract 3 the Go goroutine is parked inside `resolve` until Node's response
  arrives; say explicitly that "begin" returns when Go's final resolve is *entered*, not when it
  returns, so the two RPCs are unambiguous.
- `respond.js:425–446` assigns `event.platform` only when a route matched; the generated
  `handle` still runs for unmatched build requests (404 pages, `/_app/...` misses). State that
  begin-request does not depend on `event.platform` existing.
- The goja runtime bundle loads only universal hooks (`entry.js:27–39`), so the generated
  `hooks.server.*` never reaches the embedded engine; worth one sentence in contract 1 so an
  implementer does not add a runtime stub for it.

## Outcome

`material findings remain`
